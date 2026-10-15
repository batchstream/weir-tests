#!/usr/bin/env python3
"""Plot all crawler paths and rounds; aggregate only within one workflow run."""
import argparse
import json
from pathlib import Path
import re

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

PATHS = ('direct', 'current', 'optimized')
COLORS = dict(direct='#4c78a8', current='#f58518', optimized='#2a9d8f')
NAMES = dict(direct='Native atomic bulk', current='Current Weir', optimized='Optimized Weir')


def draw(options):
    rows, groups, output = options
    first = rows[0]
    fig, axes = plt.subplots(1, 3, figsize=(13, 3.7), constrained_layout=True)
    panels = [('fixed-rate', 'offered_products_s', 'completed_products_s', 'Offered products/s', 'Completed products/s'), ('fixed-rate', 'offered_products_s', 'db_cpu_quota_percent', 'Offered products/s', 'Database CPU (% of 1 CPU)')]
    if any(row['mode'] == 'saturation' for row in rows):
        panels.append(('saturation', 'workers', 'completed_products_s', 'Concurrent goroutine workers', 'Completed products/s'))
    else:
        panels.append(('fixed-rate', 'offered_products_s', 'sampled_db_cpu_ms_per_product', 'Offered products/s', 'Sampled DB CPU ms/completed product'))
    for axis, panel in zip(axes, panels):
        mode, xkey, ykey, xlabel, ylabel = panel
        for path in PATHS:
            subset = [row for row in rows if row['mode'] == mode and row['path'] == path and row[ykey] is not None]
            for round_index in sorted({row['round'] for row in subset}):
                points = sorted((row for row in subset if row['round'] == round_index), key=lambda row: row[xkey])
                axis.plot([row[xkey] for row in points], [row[ykey] for row in points], color=COLORS[path], alpha=0.25, linewidth=0.8)
                for row in points:
                    accepted = row['formal_measurement'] if mode == 'saturation' else row['formal_all_arrivals']
                    axis.scatter(row[xkey], row[ykey], edgecolors=COLORS[path], facecolors=COLORS[path] if accepted else 'none', marker=('o', 's', '^')[round_index % 3], s=24, linewidths=0.9)
            values = sorted((group for group in groups if group['mode'] == mode and group['path'] == path and group[ykey] is not None), key=lambda group: group[xkey])
            if values:
                axis.plot([group[xkey] for group in values], [group[ykey] for group in values], color=COLORS[path], linewidth=1.8, label=values[0].get('path_label', NAMES[path]))
        axis.set_xlabel(xlabel)
        axis.set_ylabel(ylabel)
        axis.set_ylim(bottom=0)
        axis.grid(alpha=0.2)
        if not any(row['mode'] == mode for row in rows):
            axis.text(0.5, 0.5, 'Not measured in this run', transform=axis.transAxes, ha='center', color='#666666')
    handles, labels = axes[0].get_legend_handles_labels()
    if not handles:
        handles, labels = axes[2].get_legend_handles_labels()
    fig.legend(handles, labels, loc='outside upper center', ncol=3, frameon=False)
    title = f"{first['label']} | {first['profile']} | optimized max_in_flight={first['after_max_in_flight']}"
    fig.suptitle(title, fontsize=11)
    fig.supxlabel('Thin lines/markers: every round; thick lines: completed/elapsed and CPU weighted by sampled time.\nHollow: fails fixed-rate all-arrivals gate or saturation measurement gate, or calibration. CPU includes completion drain.', fontsize=8)
    stem = re.sub('[^a-zA-Z0-9_-]+', '-', first['label'] + '-' + first['profile'])
    for extension in ('png', 'svg'):
        fig.savefig(output / (stem + '.' + extension), dpi=180, metadata=dict(Creator='crawler_evidence_plot.py'))
    plt.close(fig)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--analysis', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    data = json.loads(args.analysis.read_text())
    args.output.mkdir(parents=True, exist_ok=True)
    plt.rcParams.update({'font.size': 9, 'axes.spines.top': False, 'axes.spines.right': False, 'svg.fonttype': 'none'})
    for label, profile in sorted({(row['label'], row['profile']) for row in data['rows']}):
        rows = [row for row in data['rows'] if row['label'] == label and row['profile'] == profile]
        groups = [group for group in data['groups'] if group['label'] == label and group['profile'] == profile]
        options = rows, groups, args.output
        draw(options)


if __name__ == '__main__':
    main()
