Primary matrix at total concurrency **128**, across four independent client processes:

| Database | Write % | Direct req/s | Weir req/s | Change | Direct p99 ms | Weir p99 ms | Actual batch | DB CPU direct / Weir |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| mongo | 0 | 19,829 | 18,010 | -9.2% | 45.05 | 20.48 | 3.40 | 100.2% / 59.8% |
| mongo | 10 | 5,260 | 8,071 | +53.4% | 81.92 | 45.05 | 3.40 | 100.1% / 78.1% |
| mongo | 100 | 2,645 | 3,527 | +33.4% | 327.68 | 180.22 | 2.76 | 99.9% / 99.8% |
| search | 0 | 19,630 | 16,597 | -15.4% | 36.86 | 20.48 | 2.92 | 99.9% / 59.1% |
| search | 10 | 5,642 | 7,172 | +27.1% | 73.73 | 49.15 | 2.95 | 100.0% / 82.4% |
| search | 100 | 2,020 | 5,048 | +149.9% | 114.69 | 65.53 | 3.62 | 72.6% / 74.6% |

Same-job control at concurrency 128 (limit 1 then 32; native baseline variation is reported):

| Database | Write % | Weir limit 1 req/s | Weir limit 32 req/s | Change | Native baseline change | Batch 1 / 32 |
|---|---:|---:|---:|---:|---:|---:|
| mongo | 0 | 8,169 | 14,743 | +80.5% | +1.8% | 1.00 / 3.96 |
| mongo | 10 | 4,701 | 7,936 | +68.8% | -0.8% | 1.00 / 3.41 |
| mongo | 100 | 2,309 | 3,699 | +60.2% | +0.1% | 1.00 / 2.80 |
| search | 0 | 7,862 | 12,699 | +61.5% | +0.7% | 1.00 / 3.31 |
| search | 10 | 4,288 | 7,024 | +63.8% | -3.0% | 1.00 / 2.97 |
| search | 100 | 1,520 | 4,826 | +217.5% | +0.0% | 1.00 / 3.64 |
