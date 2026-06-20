#!/usr/bin/env python3
"""Visualise Experiment 2: IML NF convergence vs. the direct-gRPC baseline.

Reads the CSVs produced by exp2_convergence.sh and exp2_baseline.sh and writes a
single stacked horizontal bar chart that compares where the time goes in each
path:

  IML convergence (apply -> Ready) = operator schedule + driver configure
  Standalone p4runtime-shell       = gRPC pipeline push + client/container startup

Medians are plotted; the error bars are the population standard deviation of the
two totals. Output: results/resources/exp2_breakdown.png.

Usage:  python3 shared/plot_results.py
"""
import csv
import os
import statistics

import matplotlib
matplotlib.use("Agg")  # headless: write a file, never open a window
import matplotlib.pyplot as plt

HERE = os.path.dirname(os.path.abspath(__file__))
RESULTS = os.path.normpath(os.path.join(HERE, "..", "results"))
OUTDIR = os.path.join(RESULTS, "resources")


def column(path, name):
    with open(path, newline="") as f:
        return [float(row[name]) for row in csv.DictReader(f)]


def main():
    # Convergence (already in seconds).
    conv_total = column(f"{RESULTS}/converge.csv", "total_s")
    schedule = column(f"{RESULTS}/converge.csv", "schedule_s")
    configure = column(f"{RESULTS}/converge.csv", "configure_s")
    # Baseline (milliseconds -> seconds); push is the per-trial full - startup.
    full = [x / 1000 for x in column(f"{RESULTS}/baseline.csv", "full_ms")]
    startup = [x / 1000 for x in column(f"{RESULTS}/baseline.csv", "startup_ms")]
    push = [max(f - s, 0.0) for f, s in zip(full, startup)]

    med = statistics.median
    schedule_m, configure_m, conv_m = med(schedule), med(configure), med(conv_total)
    push_m, startup_m, full_m = med(push), med(startup), med(full)
    n = len(conv_total)

    fig, ax = plt.subplots(figsize=(7.2, 2.7))

    # Baseline bar (y=0): push + startup.
    ax.barh(0, push_m, color="#d62728", label="gRPC pipeline push")
    ax.barh(0, startup_m, left=push_m, color="#ff9896", label="client + container startup")
    # Convergence bar (y=1): schedule + configure.
    ax.barh(1, schedule_m, color="#1f77b4", label="operator scheduling")
    ax.barh(1, configure_m, left=schedule_m, color="#aec7e8",
            label="driver configure (2 s requeue-bound)")

    # Total labels (and the otherwise-invisible push value).
    ax.text(conv_m + 0.03, 1, f"{conv_m:.2f} s", va="center", fontsize=9)
    ax.text(full_m + 0.03, 0, f"{full_m:.2f} s  (push ≈ {push_m*1000:.0f} ms)",
            va="center", fontsize=9)

    ax.set_yticks([0, 1])
    ax.set_yticklabels(["Standalone\np4runtime-shell", "IML convergence\n(apply → Ready)"],
                       fontsize=9)
    ax.set_xlabel("time (s)")
    ax.set_xlim(0, conv_m * 1.18)
    ax.set_title(f"Experiment 2: NF convergence vs. direct-gRPC baseline (n={n}, medians)")
    ax.legend(loc="lower right", fontsize=7, framealpha=0.95)
    for side in ("top", "right"):
        ax.spines[side].set_visible(False)
    fig.tight_layout()

    os.makedirs(OUTDIR, exist_ok=True)
    out = os.path.join(OUTDIR, "exp2_breakdown.png")
    fig.savefig(out, dpi=200)
    print(f"wrote {out}")


if __name__ == "__main__":
    main()
