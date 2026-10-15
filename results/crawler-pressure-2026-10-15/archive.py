"""Archive every downloaded owned crawler artifact with two hashes and length."""
import argparse
import gzip
import hashlib
import json
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    allowed = {'crawler.log', 'crawler-observations.json', 'crawler-observations.csv', 'crawler.receipt.json', 'crawler.json', 'weir-current.receipt.json', 'weir-optimized.receipt.json', 'manifest.json', 'cleanup.json', 'node-0-config.json', 'node-0-routes.json', 'node-0-process.log', 'search-process.log', 'search-log-receipt.json', 'docker-engine.json', 'workflow-run.json'}
    files = sorted(path for path in args.source.rglob('*') if path.is_file())
    names = [path.name for path in files]
    if not files or len(set(names)) != len(names) or any(name not in allowed for name in names):
        raise ValueError('empty artifact, duplicate basenames or unexpected file')
    args.output.mkdir(parents=True, exist_ok=True)
    inventory = []
    for source in files:
        raw = source.read_bytes()
        target = args.output / (source.name + '.gz')
        if target.exists() and gzip.decompress(target.read_bytes()) != raw:
            raise ValueError('an existing archive differs: ' + str(target))
        compressed = target.read_bytes() if target.exists() else gzip.compress(raw, mtime=0)
        target.write_bytes(compressed)
        inventory.append(dict(path=target.name, gzip_sha256=hashlib.sha256(compressed).hexdigest(), sha256=hashlib.sha256(raw).hexdigest(), bytes=len(raw)))
    if {path.name for path in args.output.glob('*.gz')} != {item['path'] for item in inventory}:
        raise ValueError('existing output has files absent from this artifact')
    (args.output / 'files.json').write_text(json.dumps(inventory, indent=2) + '\n')
    print(json.dumps(dict(files=len(inventory), uncompressed_bytes=sum(item['bytes'] for item in inventory))))


if __name__ == '__main__':
    main()
