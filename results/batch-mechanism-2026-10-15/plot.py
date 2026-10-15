#!/usr/bin/env python3
"""Plot each formal workflow separately, showing all rounds and failed gates."""
import argparse
import json
from pathlib import Path

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

PATHS = ('direct', 'current', 'optimized')
COLORS = ('#4c78a8', '#f58518', '#2a9d8f')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--analysis', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    rows = json.loads(args.analysis.read_text())['rows']
    args.output.mkdir(parents=True, exist_ok=True)
    plt.rcParams.update({'font.size': 9, 'axes.spines.top': False, 'axes.spines.right': False, 'svg.fonttype': 'none'})
    for label in sorted({row['label'] for row in rows if row['role'] == 'formal'}):
        selected = [row for row in rows if row['label'] == label]
        unit = 'partial update' if selected[0]['profile'] == 'partial-update' else 'business product'
        fig, axes = plt.subplots(1, 4, figsize=(15, 4.1))
        panels = [('completed_products_s', f'Completed {unit}s/s'), ('sampled_db_cpu_ms_per_product', f'Sampled DB CPU ms/completed {unit}'), ('p99_planned_to_visible_ms', 'Planned-to-visible p99 upper bound (ms)'), ('search_bulk_wire_records_mean', 'Attempted framed records/bulk request')]
        for axis, (field, title) in zip(axes, panels):
            for path_index, path in enumerate(PATHS):
                values = [row for row in selected if row['path'] == path and row.get(field) is not None]
                for row in values:
                    position = path_index + (row['round']-2)*.14
                    axis.scatter(position, row[field], marker=('o', 's', '^')[row['round']-1], edgecolors=COLORS[path_index], facecolors=COLORS[path_index] if row['formal_all_arrivals'] else 'none', s=35)
            axis.set_xticks(range(3), ('Native', 'Weir\n0 ms', 'Weir\n20 ms'))
            axis.set_xlim(-.45, 2.45)
            axis.set_ylim(bottom=0)
            axis.set_ylabel(title)
            axis.grid(alpha=.2)
        fig.suptitle(f"{selected[0]['profile']} | offered {selected[0]['offered_products_s']} {unit}s/s | 512 workers / 4 OS processes", fontsize=11)
        fig.supxlabel('Circles: round 1; squares: round 2; triangles: round 3. Hollow markers fail all-arrivals gate. Throughput and CPU include drain.\nNative sends single updates; framed record counts are attempted request sizes, not acknowledged writes.', fontsize=8)
        fig.tight_layout(rect=(0, .07, 1, .94))
        for extension in ('png', 'svg'):
            target = args.output/(label+'.'+extension)
            fig.savefig(target, dpi=180, bbox_inches='tight', metadata={'Creator': 'batch-mechanism plot.py'})
            if extension == 'svg':
                target.write_text('\n'.join(line.rstrip() for line in target.read_text().splitlines())+'\n')
        plt.close(fig)


if __name__ == '__main__':
    main()
