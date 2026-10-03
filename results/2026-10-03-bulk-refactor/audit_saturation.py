#!/usr/bin/env python3
"""Independent raw saturation evidence audit. Only read explicitly named public receipts."""
import argparse, json, math, pathlib, re, sys
parser=argparse.ArgumentParser()
parser.add_argument("root",type=pathlib.Path)
parser.add_argument("--head",required=True)
parser.add_argument("--server",required=True)
parser.add_argument("--sdk",default="v0.4.2")
parser.add_argument("--protocol",default="v0.2.1")
parser.add_argument("--server-version")
parser.add_argument("--server-sum")
parser.add_argument("--quota",type=float,default=1)
parser.add_argument("--legacy-baseline",action="store_true")
parser.add_argument("--output",type=pathlib.Path)
parser.add_argument("--baseline-notes",type=pathlib.Path)
args=parser.parse_args()
root=args.root
head=args.head
weir=args.server
assert root.is_dir() and math.isfinite(args.quota) and args.quota>0
summary={"source":head,"weir_source":weir,"sdk":args.sdk,"protocol":args.protocol,"database_cpus":args.quota,"legacy_baseline":args.legacy_baseline,"reports":[]}
all_successes=0
all_coverage=[]
all_stage_count=0
client_hashes=set()
weir_hashes=set()
seen_receipts=set()
seen_manifests=set()
seen_cleanups=set()
def close(a,b): assert math.isclose(a,b,rel_tol=1e-10,abs_tol=1e-9),(a,b)
def qualify(res, threshold):
    samples=res["samples"]; limit=res["measurement_elapsed_ns"]; allocation=res["database_allocated_cpus"]
    close(allocation,args.quota)
    intervals=0; valid=0; weighted=0.0; full=0
    for left,right in zip(samples,samples[1:]):
        span=min(right["elapsed_ns"],limit)-max(left["elapsed_ns"],0)
        if span<=0: continue
        clock=right["elapsed_ns"]-left["elapsed_ns"]
        if left.get("database_cpu_counter_clock_unix_ns",0)>0 and right.get("database_cpu_counter_clock_unix_ns",0)>0:
            clock=right["database_cpu_counter_clock_unix_ns"]-left["database_cpu_counter_clock_unix_ns"]
        if clock<=0 or left.get("error") or right.get("error") or "database_cpu_time_ns" not in left or "database_cpu_time_ns" not in right: continue
        delta=right["database_cpu_time_ns"]-left["database_cpu_time_ns"]
        if delta<0: continue
        cpu=delta/clock/allocation*100
        close(right["database_cpu_budget_percent"],cpu)
        intervals+=1; valid+=span; weighted+=cpu*span
        if cpu>=threshold: full+=span
    mean=weighted/valid if valid else 0
    coverage=valid/limit if valid and limit else 0
    fraction=full/limit if valid and limit else 0
    sat=intervals>=5 and coverage>=.8 and mean>=threshold and fraction>=.8
    assert intervals==res["valid_cpu_intervals"] and valid==res["valid_cpu_sampling_elapsed_ns"]
    close(mean,res["mean_database_cpu_budget_percent"]);close(coverage,res["cpu_sampling_time_coverage"]);close(fraction,res["fraction_intervals_at_cpu_threshold"])
    assert sat==res["sustained_cpu_saturation"]
    return sat,mean,coverage,fraction
def metric_sum(snapshot,name,wanted):
    total=0.0; found=False
    assert not snapshot.get("error")
    for line in snapshot["raw"].splitlines():
        match=re.fullmatch(r"([A-Za-z_:][A-Za-z0-9_:]*)(?:\{(.*)\})?\s+(\S+)(?:\s+[0-9]+)?",line)
        if not match or match[1]!=name: continue
        labels={key:json.loads(value) for key,value in re.findall(r'([A-Za-z_][A-Za-z0-9_]*)=("(?:[^"\\]|\\.)*")',match[2] or "")}
        if all(labels.get(key)==value for key,value in wanted.items()):
            total+=float(match[3]); found=True
    assert found,("raw metric absent",name,wanted)
    return total

def load_json(path):
    return json.loads(path.read_text())
def fixture_evidence(file, report):
    directory=pathlib.Path(report["provenance"]["fixture_logs"]).name
    candidates=[p for p in root.rglob("manifest.json") if p.parent.name==directory]
    assert len(candidates)==1,("fixture manifest identity",directory,len(candidates))
    manifest_file=candidates[0]
    manifest=load_json(manifest_file)
    cleanup_file=manifest_file.parent/"cleanup.json"
    cleanup=load_json(cleanup_file)
    assert manifest["host_os"]=="linux" and manifest["host_arch"]=="amd64"
    assert manifest["owner"]==cleanup["owner"] and not cleanup.get("errors")
    containers={item["id"]:item for item in manifest["containers"]}
    removed={item["id"]:item for item in cleanup["containers"]}
    assert set(containers)==set(removed) and len(containers)==2
    assert all(item["removed"] for item in removed.values())
    assert len(manifest["nodes"])==1 and manifest["nodes"][0]["Owner"] is True
    assert len(cleanup["processes"])==1 and all(item["stopped"] for item in cleanup["processes"])
    opts=manifest["options"]
    assert opts["Backends"]==["mongo","search"] and opts["OwnerCount"]==1
    assert opts["StoreConcurrency"]==32 and opts["BatchSize"]==32 and opts["IngressSessions"]==64
    close(opts["DatabaseCPUs"],args.quota)
    provenance=report["provenance"]
    close(float(provenance["database_docker_cpu_quota"]),args.quota)
    assert provenance["weir_binary_sha256"]==manifest["binary_sha256"]
    assert len(manifest["binary_sha256"])==64
    weir_hashes.add(manifest["binary_sha256"])
    assert all(item["name"].startswith("weir-tests-"+manifest["owner"]+"-") for item in containers.values())
    for backend,pin in (("mongo","mongodb_image"),("search","elasticsearch_image")):
        matching=[item for item in containers.values() if item["backend"]==backend]
        assert len(matching)==1 and matching[0]["image"]==provenance[pin]
    config=load_json(manifest_file.parent/"node-0-config.json")
    routes=load_json(manifest_file.parent/"node-0-routes.json")
    assert config["transport"]=={"max_connections":64,"max_sessions":64}
    stores={item["name"]:item for item in routes["stores"]}
    assert set(stores)=={"mongo","search"}
    for store in stores.values():
        assert store["max_concurrency"]==32 and store["max_batch_operations"]==32 and store["max_read_size"]=="2MiB"
    if not args.legacy_baseline:
        backend=report["parameters"]["dataset"]["backend"]
        quota=report["database_cpu_quota"]
        assert set(quota)=={"container_id","host_config_nano_cpus","requested_cpus","observed_cpus"}
        assert quota["container_id"] in containers and containers[quota["container_id"]]["backend"]==backend
        assert quota["host_config_nano_cpus"]==int(args.quota*1e9)
        close(quota["requested_cpus"],args.quota)
        close(quota["observed_cpus"],quota["host_config_nano_cpus"]/1e9)
        workspace={"mongo":max(384,32*41),"search":max(384,32*96)}
        assert opts["WorkingMemoryMiB"]==workspace
        memory=(64*96+1024+sum(workspace.values())+len(workspace)*32*2+1023)//1024*1024
        assert opts["ProcessMemoryMiB"]==memory and config["memory"]==str(memory)+"MiB"
        assert provenance["weir_memory_budget"]==str(memory)+"MiB (declared process admission budget; not an OS reservation)"
        for backend,working in workspace.items():
            assert stores[backend]["working_memory"]==str(working)+"MiB"
            assert provenance["weir_"+backend+"_working_memory_mib"]==str(working)
        # Cross-check documented production Config.ReservedMemory envelope.
        required=64+64*96+64*.25+sum(32+32+working+32*2 for working in workspace.values())
        assert memory>=required
    else:
        assert "database_cpu_quota" not in report
        workspace={"mongo":384,"search":384}
    seen_manifests.add(manifest_file.resolve())
    seen_cleanups.add(cleanup_file.resolve())
    return manifest,workspace
def metric_delta(metrics,name,wanted):
    left=metric_sum(metrics["before"],name,wanted)
    right=metric_sum(metrics["after"],name,wanted)
    assert right>=left,("counter reset",name,wanted)
    return right-left
def process_cores(resources,key):
    weighted=0.;valid=0;limit=resources["measurement_elapsed_ns"]
    for left,right in zip(resources["samples"],resources["samples"][1:]):
        span=min(right["elapsed_ns"],limit)-max(left["elapsed_ns"],0)
        clock=right["elapsed_ns"]-left["elapsed_ns"]
        if span<=0 or clock<=0 or key not in left or key not in right: continue
        delta=right[key]-left[key]
        if delta<0: continue
        weighted+=delta/clock*span
        valid+=span
    return {"approximate_cores":weighted/valid if valid else None,"coverage":valid/limit if limit else 0}

receipts=list(root.rglob("measurement.receipt.json"))
for file in receipts:
    receipt=json.loads(file.read_text())
    assert receipt["source"]==head and receipt["source_dirty"] is False
    assert receipt["versions"]["weir_source"]==weir and receipt["versions"]["sdk"]==args.sdk and receipt["versions"]["protocol"]==args.protocol
    assert "vcs.revision="+head in receipt["client_build"] and "vcs.modified=false" in receipt["client_build"]
    assert "github.com/batchstream/weir-go\t"+args.sdk in receipt["client_build"]
    command=receipt["command"]; options=dict(zip(command[1::2],command[2::2]))
    for key,value in {"-mode":"saturation","-database-cpus":format(args.quota,"g"),"-store-concurrency":"32","-concurrency-levels":"8,32,64","-batch-sizes":"32","-records":"2048","-payload-bytes":"1024","-rounds":"3","-warmup-duration":"10s","-duration":"20s"}.items(): assert options[key]==value
    if args.server_version: assert receipt["versions"]["weir_version"]==args.server_version
    if args.server_sum: assert receipt["versions"]["weir_sum"]==args.server_sum
    client_hashes.add(receipt["client_binary_sha256"])
    assert len(receipt["client_binary_sha256"])==64
    seen_receipts.add(file.resolve())
    print("RECEIPT",file.relative_to(root),options["-write-percent"],receipt["client_binary_sha256"])
reports=[]
for name in ("mongo.json","search.json"):
    for file in root.rglob(name):
        report=json.loads(file.read_text())
        if "database_saturated_comparisons" not in report: continue
        reports.append(file)
        params=report["parameters"]; threshold=params["database_cpu_budget_threshold_percent"]
        assert params["concurrency_levels"]==[8,32,64] and params["batch_sizes"]==[32] and params["paired_rounds"]==3
        assert not report.get("incomplete") and len(report["pairs"])==9
        assert report["provenance"]["test_source"]==head and report["provenance"]["test_source_dirty"]=="false" and report["provenance"]["weir_source"]==weir
        assert report["provenance"]["sdk"]==args.sdk and report["provenance"]["protocol"]==args.protocol
        manifest,workspace=fixture_evidence(file,report)
        receipt_file=file.parent.parent/"measurement.receipt.json"
        assert receipt_file.resolve() in seen_receipts
        cpu={}; maximum_error=0; batch_averages=[]
        diagnostic_points=[]
        for pair in report["pairs"]:
            expected=["direct","weir"] if ([8,32,64].index(pair["concurrency"])+pair["round"]-1)%2==0 else ["weir","direct"]
            assert pair["order"]==expected
            for path in ("direct","weir"):
                result=pair[path]
                assert result["verified"] and not result.get("verification_error")
                assert result["planned"]==result["attempted"]==result["succeeded"]>0
                assert result["errors"]==result["indeterminate"]==result["applied_with_error"]==result["not_attempted"]==0
                assert result["succeeded"]==32*result["client_batch_requests"]==result["reads"]+result["writes"]
                assert result["elapsed_ns"]>=20_000_000_000
                close(result["successful_operations_per_second"],result["succeeded"]*1e9/result["elapsed_ns"])
                assert result["resources"]["measurement_elapsed_ns"]==result["elapsed_ns"]
                cpu[(pair["concurrency"],pair["round"],path)]=qualify(result["resources"],threshold)
                all_successes+=result["succeeded"]
                all_stage_count+=1
                all_coverage.append(cpu[(pair["concurrency"],pair["round"],path)][2])
                for sample in result["resources"]["samples"]:
                    assert not sample.get("error"),("monitor failure",file,pair["concurrency"],pair["round"],path)
                if path=="weir":
                    assert result["evidence"]["protocol"]=="gRPC unary Read/Mutate batches via published Weir SDK"
                    metrics=result["server_metrics"]; assert not metrics.get("unavailable")
                    delta=metrics["timed_counter_deltas"]
                    for key,value in delta.items():
                        if key.startswith("weir_rpc_completions_total:"):
                            name,method=key.split(":",1); wanted={"method":method}
                        else:
                            name=key; wanted={"store":params["dataset"]["store"]}
                        left=metric_sum(metrics["before"],name,wanted); right=metric_sum(metrics["after"],name,wanted)
                        assert right>=left
                        close(value,right-left)
                    close(metrics["timed_adapter_batch_average"],32)
                    close(delta["weir_store_batch_operations_count"],result["client_batch_requests"])
                    close(delta["weir_store_batch_operations_sum"],result["succeeded"])
                    close(delta["weir_store_executions_total"],result["client_batch_requests"])
                    close(delta["weir_store_records_total"],result["succeeded"])
                    assert delta["weir_store_rejections_total"]==0
                    assert metrics["configured_backend_concurrency_limit"] == 32
                    close(metric_sum(metrics["before"],"weir_store_concurrency_limit",{"store":params["dataset"]["store"]}),32)
                    close(metric_sum(metrics["after"],"weir_store_concurrency_limit",{"store":params["dataset"]["store"]}),32)
                    for method, count in (("read",result["reads"]/32),("mutate",result["writes"]/32)):
                        if count: close(delta["weir_rpc_completions_total:"+method],count)
                    for method,count in (("read",result["reads"]/32),("mutate",result["writes"]/32)):
                        labels={"listener":"application","method":method,"status":"ok"}
                        close(metric_delta(metrics,"weir_rpc_completions_total",labels),count)
                        for status in ("canceled","deadline","non_ok"):
                            labels["status"]=status
                            close(metric_delta(metrics,"weir_rpc_completions_total",labels),0)
                        close(metric_delta(metrics,"weir_rpc_completions_total",{"listener":"peer","method":method}),0)
                    close(metric_delta(metrics,"weir_store_records_total",{"store":params["dataset"]["store"],"operation":"read","outcome":"success"}),result["reads"])
                    close(metric_delta(metrics,"weir_store_records_total",{"store":params["dataset"]["store"],"operation":"mutate","outcome":"applied"}),result["writes"])
                    close(metric_delta(metrics,"weir_store_batch_operations_bucket",{"store":params["dataset"]["store"],"le":"16"}),0)
                    for field,metric in (("timed_queue_wait_mean_seconds","weir_store_queue_wait_seconds"),("timed_adapter_execution_mean_seconds","weir_store_execution_seconds")):
                        close(metrics[field],metric_delta(metrics,metric+"_sum",{"store":params["dataset"]["store"]})/metric_delta(metrics,metric+"_count",{"store":params["dataset"]["store"]}))
                    for snapshot in ("before","after"):
                        close(metric_sum(metrics[snapshot],"weir_store_working_reserved_bytes_limit",{"store":params["dataset"]["store"]}),workspace[params["dataset"]["backend"]]*(1<<20))
                        close(metric_sum(metrics[snapshot],"weir_store_working_reserved_bytes",{"store":params["dataset"]["store"]}),0)
                        close(metric_sum(metrics[snapshot],"weir_store_active_executions",{"store":params["dataset"]["store"]}),0)
                        close(metric_sum(metrics[snapshot],"weir_ingress_sessions",{}),0)
                    batch_averages.append(metrics["timed_adapter_batch_average"])
        points={}
        for path in ("direct","weir"):
            for workers in params["concurrency_levels"]:
                stages=[p[path] for p in report["pairs"] if p["concurrency"]==workers]
                diagnostic={"path":path,"concurrency":workers}
                for counter in ("client_cpu_time_ns","weir_cpu_time_ns"):
                    observations=[process_cores(stage["resources"],counter) for stage in stages]
                    observed=[item["approximate_cores"] for item in observations if item["approximate_cores"] is not None]
                    diagnostic[counter.replace("_time_ns","_approximate_cores")]=sum(observed)/len(observed) if observed else None
                if path=="weir":
                    diagnostic["queue_wait_mean_ms"]=1000*sum(stage["server_metrics"]["timed_queue_wait_mean_seconds"] for stage in stages)/len(stages)
                    diagnostic["adapter_execution_mean_ms"]=1000*sum(stage["server_metrics"]["timed_adapter_execution_mean_seconds"] for stage in stages)/len(stages)
                diagnostic_points.append(diagnostic)
        for path in ("direct","weir"):
            for workers in params["concurrency_levels"]:
                selected=[p[path] for p in report["pairs"] if p["concurrency"]==workers]
                rate=sum(x["succeeded"] for x in selected)*1e9/sum(x["elapsed_ns"] for x in selected)
                full=all(cpu[(workers,p["round"],path)][0] for p in report["pairs"] if p["concurrency"]==workers)
                mean=sum(cpu[(workers,p["round"],path)][1] for p in report["pairs"] if p["concurrency"]==workers)/len(selected)
                points[(path,workers)]={"rate":rate,"full":full,"mean":mean}
            for i,workers in enumerate(params["concurrency_levels"]):
                point=points[(path,workers)]
                plateau=i+1<len(params["concurrency_levels"]) and points[(path,params["concurrency_levels"][i+1])]["rate"]<=point["rate"]*1.1
                point["plateau"]=plateau; point["sat"]=point["full"] and plateau
        for point in report["points"]:
            recalculated=points[(point["path"],point["concurrency"])]
            close(point["successful_operations_per_second"],recalculated["rate"]);close(point["mean_database_cpu_budget_percent"],recalculated["mean"])
            assert point["all_rounds_successful_verified"] and point["all_rounds_cpu_saturated"]==recalculated["full"]
            assert point["throughput_plateau_at_next_concurrency"]==recalculated["plateau"] and point["database_saturation_demonstrated"]==recalculated["sat"]
        selected={}
        for path in ("direct","weir"):
            candidates=[(w,points[(path,w)]) for w in params["concurrency_levels"] if points[(path,w)]["sat"]]
            if candidates: selected[path]=max(candidates,key=lambda p:p[1]["rate"])
        comparison=report["database_saturated_comparisons"][0]
        if len(selected)==2:
            ratio=selected["weir"][1]["rate"]/selected["direct"][1]["rate"]
            close(comparison["weir_direct_saturated_throughput_ratio"],ratio)
            close(comparison["weir_saturated_throughput_delta_percent"],100*(ratio-1))
            for path in ("direct","weir"): assert comparison[path+"_database_saturated_point"]["concurrency"]==selected[path][0]
            print("COMPARISON",file.relative_to(root),"ratio",ratio,"selected",selected)
        else:
            assert "weir_direct_saturated_throughput_ratio" not in comparison and comparison.get("unavailable")
            print("UNAVAILABLE",file.relative_to(root),"saturated",selected)
        summary["reports"].append({"backend":params["dataset"]["backend"],"write_percent":params["requested_write_percent"],"quota":args.quota,"selected":selected,"ratio":comparison.get("weir_direct_saturated_throughput_ratio"),"delta_percent":comparison.get("weir_saturated_throughput_delta_percent"),"unavailable":comparison.get("unavailable"),"points":[{"path":key[0],"concurrency":key[1],**value} for key,value in points.items()],"diagnostics":diagnostic_points,"workspace_mib":workspace,"weir_binary_sha256":manifest["binary_sha256"],"file":str(file.relative_to(root))})
        print("POINTS",json.dumps({str(k):v for k,v in points.items()},sort_keys=True))
        print("CPU_COVERAGE_RANGE",min(x[2] for x in cpu.values()),max(x[2] for x in cpu.values()),"FULL_CPU_TIME_RANGE",min(x[3] for x in cpu.values()),max(x[3] for x in cpu.values()),"BATCH_RANGE",min(batch_averages),max(batch_averages))
cleanups=list(root.rglob("cleanup.json"))
for file in cleanups:
    cleanup=json.loads(file.read_text())
    assert not cleanup.get("errors")
    assert all(p["stopped"] for p in cleanup.get("processes",[]))
    assert all(c["removed"] for c in cleanup.get("containers",[]))
    print("CLEANUP",file.relative_to(root),cleanup.get("owner"))
assert reports
print("AUDITED_REPORTS",len(reports),"RECEIPTS",len(receipts),"CLEANUPS",len(cleanups))

assert len(reports)==6 and len(receipts)==3 and len(cleanups)==3
print("ALL108TIMEDSTAGES PASS")

assert len(client_hashes)==len(weir_hashes)==1
assert len(seen_manifests)==len(seen_cleanups)==3
summary["successful_operations"]=all_successes
summary["timed_paths"]=all_stage_count
summary["cpu_sampling_coverage_min"]=min(all_coverage)
summary["cpu_sampling_coverage_max"]=max(all_coverage)
summary["client_binary_sha256"]=next(iter(client_hashes))
summary["weir_binary_sha256"]=next(iter(weir_hashes))
summary["limitations"]=["Adapter batch metrics count 32 logical records per invocation; they do not prove identical database wire commands or acknowledgement encoding.","Native Mongo Collection.BulkWrite aggregate ACK differs from Weir Mongo admin bulkWrite verbose per-item ACK.","Client/Weir CPU is approximate from ps cumulative process counters.","Unavailable capacity comparisons must remain unavailable even when measured throughput improves."]
if args.legacy_baseline:
    summary["limitations"].append("Historical baseline does not archive actual NanoCpus inspect evidence; do not infer it was independently archived.")
if args.baseline_notes:
    baseline=load_json(args.baseline_notes)
    close(baseline["database_cpus"],args.quota)
    old={(item["backend"],item["write_percent"]):item for item in baseline["reports"]}
    assert len(old)==6 and len(summary["reports"])==6
    compared=[]
    for item in summary["reports"]:
        previous=old[(item["backend"],item["write_percent"])]
        assert previous["quota"]==item["quota"]
        rates={(point["path"],point["concurrency"]):point for point in previous["points"]}
        matched=[]
        for point in item["points"]:
            earlier=rates[(point["path"],point["concurrency"])]
            matched.append({"path":point["path"],"concurrency":point["concurrency"],"baseline_rate":earlier["rate"],"final_rate":point["rate"],"measured_rate_delta_percent":100*(point["rate"]/earlier["rate"]-1),"baseline_cpu_budget_percent":earlier["mean"],"final_cpu_budget_percent":point["mean"],"baseline_capacity_demonstrated":earlier["sat"],"final_capacity_demonstrated":point["sat"]})
        comparison={"backend":item["backend"],"write_percent":item["write_percent"],"database_cpus":args.quota,"baseline_ratio":previous["ratio"],"final_ratio":item["ratio"],"points":matched}
        if previous["ratio"] is not None and item["ratio"] is not None:
            comparison["capacity_ratio_change_percent"]=100*(item["ratio"]/previous["ratio"]-1)
        compared.append(comparison)
    summary["baseline_source"]=baseline["source"]
    summary["baseline_weir_source"]=baseline["weir_source"]
    summary["baseline_comparison"]=compared
if args.output:
    args.output.write_text(json.dumps(summary,indent=2,sort_keys=True)+"\n")
print("AUDIT_SUMMARY",json.dumps(summary,sort_keys=True))
