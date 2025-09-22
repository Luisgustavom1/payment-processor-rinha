#!/usr/bin/env python3
"""
Payment Processor Results Analyzer - Simple Graphics Version

This script reads all JSON files from the results directory and creates
visualizations showing the evolution of key performance metrics using
version numbers on the x-axis. Only generates graphics, no text files.
"""

import json
import os
import glob
import re
from typing import List, Dict, Any
import matplotlib.pyplot as plt

def extract_version_from_filename(filename: str) -> str:
    """Extract version number from filename."""
    match = re.search(r'v(\d+\.\d+)', filename)
    return match.group(1) if match else "unknown"

def version_sort_key(version_str: str):
    """Create a sort key for version strings."""
    try:
        parts = version_str.split('.')
        return [int(part) for part in parts]
    except:
        return [0, 0]

def load_results_data(results_dir: str = "../results") -> List[Dict[str, Any]]:
    """Load all JSON files from the results directory."""
    data = []

    # Find all JSON files in the results directory
    json_files = glob.glob(os.path.join(results_dir, "**/*.json"), recursive=True)

    if not json_files:
        print(f"No JSON files found in {results_dir}")
        return data

    print(f"Found {len(json_files)} JSON files:")

    for file_path in sorted(json_files):
        try:
            with open(file_path, 'r', encoding='utf-8') as f:
                result = json.load(f)

            # Add metadata
            result['filename'] = os.path.basename(file_path)
            result['version'] = extract_version_from_filename(file_path)

            data.append(result)
            print(f"  ✓ {file_path}")

        except Exception as e:
            print(f"  ✗ Error loading {file_path}: {e}")

    # Sort by version number
    data.sort(key=lambda x: version_sort_key(x.get('version', '0.0')))

    return data

def create_evolution_plots(data: List[Dict[str, Any]]):
    """Create multiple plots showing the evolution of key metrics."""

    if not data:
        print("No data to plot!")
        return

    # Set up the plot style
    plt.style.use('default')
    colors = ['#1f77b4', '#ff7f0e', '#2ca02c', '#d62728', '#9467bd', '#8c564b']

    # Create figure with subplots
    fig, axes = plt.subplots(3, 2, figsize=(15, 18))
    fig.suptitle('Payment Processor Results Evolution', fontsize=16, fontweight='bold')

    # Prepare data for plotting
    versions = [item['version'] for item in data]
    version_labels = [f"v{v}" for v in versions]

    # 1. Total Líquido Evolution
    total_liquido = [item.get('total_liquido', 0) for item in data]
    axes[0, 0].plot(range(len(versions)), total_liquido, marker='o', linewidth=3, markersize=10, color=colors[0])
    axes[0, 0].set_title('Total Líquido Evolution (Final Score)', fontweight='bold', fontsize=12)
    axes[0, 0].set_ylabel('Total Líquido', fontweight='bold')
    axes[0, 0].set_xlabel('Version', fontweight='bold')
    axes[0, 0].set_xticks(range(len(versions)))
    axes[0, 0].set_xticklabels(version_labels)
    axes[0, 0].grid(True, alpha=0.3)

    # Add value labels
    for i, y in enumerate(total_liquido):
        axes[0, 0].annotate(f'{y:,.0f}', (i, y), textcoords="offset points",
                           xytext=(0,15), ha='center', fontsize=10,
                           bbox=dict(boxstyle="round,pad=0.3", facecolor='white', alpha=0.8))

    # 2. P99 Response Time Evolution
    p99_values = []
    for item in data:
        p99_str = item.get('p99', {}).get('valor', '0ms')
        # Extract numeric value from string like "545.84ms"
        p99_num = float(re.search(r'(\d+\.?\d*)', p99_str).group(1)) if re.search(r'(\d+\.?\d*)', p99_str) else 0
        p99_values.append(p99_num)

    axes[0, 1].plot(range(len(versions)), p99_values, marker='s', linewidth=3, markersize=10, color=colors[1])
    axes[0, 1].set_title('P99 Response Time Evolution', fontweight='bold', fontsize=12)
    axes[0, 1].set_ylabel('P99 (ms)', fontweight='bold')
    axes[0, 1].set_xlabel('Version', fontweight='bold')
    axes[0, 1].set_xticks(range(len(versions)))
    axes[0, 1].set_xticklabels(version_labels)
    axes[0, 1].grid(True, alpha=0.3)

    for i, y in enumerate(p99_values):
        axes[0, 1].annotate(f'{y:.1f}ms', (i, y), textcoords="offset points",
                           xytext=(0,15), ha='center', fontsize=10,
                           bbox=dict(boxstyle="round,pad=0.3", facecolor='white', alpha=0.8))

    # 3. Payment Success Rate
    success_rates = []
    for item in data:
        success = item.get('pagamentos_solicitados', {}).get('qtd_sucesso', 0)
        failure = item.get('pagamentos_solicitados', {}).get('qtd_falha', 0)
        total_requests = success + failure
        rate = (success / total_requests * 100) if total_requests > 0 else 0
        success_rates.append(rate)

    axes[1, 0].plot(range(len(versions)), success_rates, marker='^', linewidth=3, markersize=10, color=colors[2])
    axes[1, 0].set_title('Payment Success Rate Evolution', fontweight='bold', fontsize=12)
    axes[1, 0].set_ylabel('Success Rate (%)', fontweight='bold')
    axes[1, 0].set_xlabel('Version', fontweight='bold')
    axes[1, 0].set_xticks(range(len(versions)))
    axes[1, 0].set_xticklabels(version_labels)
    axes[1, 0].set_ylim(0, 105)
    axes[1, 0].grid(True, alpha=0.3)

    for i, y in enumerate(success_rates):
        axes[1, 0].annotate(f'{y:.1f}%', (i, y), textcoords="offset points",
                           xytext=(0,15), ha='center', fontsize=10,
                           bbox=dict(boxstyle="round,pad=0.3", facecolor='white', alpha=0.8))

    # 4. Inconsistencies Evolution
    inconsistencies = [item.get('multa', {}).get('composicao', {}).get('num_inconsistencias', 0) for item in data]
    axes[1, 1].plot(range(len(versions)), inconsistencies, marker='D', linewidth=3, markersize=10, color=colors[3])
    axes[1, 1].set_title('Number of Inconsistencies Evolution', fontweight='bold', fontsize=12)
    axes[1, 1].set_ylabel('Number of Inconsistencies', fontweight='bold')
    axes[1, 1].set_xlabel('Version', fontweight='bold')
    axes[1, 1].set_xticks(range(len(versions)))
    axes[1, 1].set_xticklabels(version_labels)
    axes[1, 1].grid(True, alpha=0.3)

    for i, y in enumerate(inconsistencies):
        axes[1, 1].annotate(f'{y}', (i, y), textcoords="offset points",
                           xytext=(0,15), ha='center', fontsize=10,
                           bbox=dict(boxstyle="round,pad=0.3", facecolor='white', alpha=0.8))

    # 5. Lag Evolution
    lags = [item.get('lag', {}).get('lag', 0) for item in data]
    axes[2, 0].plot(range(len(versions)), lags, marker='v', linewidth=3, markersize=10, color=colors[4])
    axes[2, 0].set_title('Payment Processing Lag Evolution', fontweight='bold', fontsize=12)
    axes[2, 0].set_ylabel('Lag (requests)', fontweight='bold')
    axes[2, 0].set_xlabel('Version', fontweight='bold')
    axes[2, 0].set_xticks(range(len(versions)))
    axes[2, 0].set_xticklabels(version_labels)
    axes[2, 0].axhline(y=0, color='black', linestyle='--', alpha=0.5, linewidth=2)
    axes[2, 0].grid(True, alpha=0.3)

    for i, y in enumerate(lags):
        axes[2, 0].annotate(f'{y}', (i, y), textcoords="offset points",
                           xytext=(0,15), ha='center', fontsize=10,
                           bbox=dict(boxstyle="round,pad=0.3", facecolor='white', alpha=0.8))

    # 6. Total Bruto vs Total Líquido Comparison
    total_bruto = [item.get('total_bruto', 0) for item in data]
    width = 0.35
    x_pos = range(len(versions))

    bars1 = axes[2, 1].bar([x - width/2 for x in x_pos], total_bruto, width,
                          label='Total Bruto', alpha=0.8, color=colors[0])
    bars2 = axes[2, 1].bar([x + width/2 for x in x_pos], total_liquido, width,
                          label='Total Líquido', alpha=0.8, color=colors[1])

    axes[2, 1].set_title('Total Bruto vs Total Líquido Comparison', fontweight='bold', fontsize=12)
    axes[2, 1].set_ylabel('Amount', fontweight='bold')
    axes[2, 1].set_xlabel('Version', fontweight='bold')
    axes[2, 1].set_xticks(x_pos)
    axes[2, 1].set_xticklabels(version_labels)
    axes[2, 1].legend()
    axes[2, 1].grid(True, alpha=0.3)

    # Add value labels on bars
    for bar in bars1:
        height = bar.get_height()
        axes[2, 1].text(bar.get_x() + bar.get_width()/2., height,
                       f'{height:,.0f}', ha='center', va='bottom', fontsize=8)

    for bar in bars2:
        height = bar.get_height()
        axes[2, 1].text(bar.get_x() + bar.get_width()/2., height,
                       f'{height:,.0f}', ha='center', va='bottom', fontsize=8)

    # Adjust layout
    plt.tight_layout()
    plt.subplots_adjust(top=0.95)

    # Save the plot
    plt.savefig('payment_processor_evolution.png', dpi=300, bbox_inches='tight')
    print("✓ Evolution plots saved as 'payment_processor_evolution.png'")

    return fig

def print_summary(data: List[Dict[str, Any]]):
    """Print a simple console summary."""
    if not data:
        return

    print(f"\n📊 Summary of {len(data)} results:")
    print("-" * 50)

    for item in data:
        version = item['version']
        total_liquido = item.get('total_liquido', 0)
        p99_str = item.get('p99', {}).get('valor', '0ms')
        p99_num = float(re.search(r'(\d+\.?\d*)', p99_str).group(1)) if re.search(r'(\d+\.?\d*)', p99_str) else 0

        success = item.get('pagamentos_solicitados', {}).get('qtd_sucesso', 0)
        failure = item.get('pagamentos_solicitados', {}).get('qtd_falha', 0)
        total_requests = success + failure
        success_rate = (success / total_requests * 100) if total_requests > 0 else 0

        print(f"v{version}: Líquido={total_liquido:,.0f}, P99={p99_num:.1f}ms, Success={success_rate:.1f}%")

def main():
    """Main function to run the analysis."""
    print("🔍 Payment Processor Results Analyzer - Simple Graphics")
    print("=" * 60)

    # Load data
    data = load_results_data()

    if not data:
        print("❌ No data found to analyze!")
        return

    print(f"\n✅ Loaded {len(data)} result files")

    # Print simple summary
    print_summary(data)

    # Create visualizations
    print("\n📊 Creating evolution plots...")
    create_evolution_plots(data)

    # Show plots
    print("\n🎯 Opening visualization...")
    plt.show()

    print("\n✅ Analysis complete!")
    print(f"📁 Generated file: payment_processor_evolution.png")

if __name__ == "__main__":
    # Check for required libraries
    try:
        import matplotlib.pyplot as plt
    except ImportError as e:
        print(f"❌ Missing required library: {e}")
        print("\nTo install required dependencies, run:")
        print("pip install matplotlib")
        exit(1)

    main()
