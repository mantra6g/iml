#!/bin/bash

# Script to compile P4 programs for BMv2
# Usage: ./compile.sh <p4_file>

if [ $# -ne 1 ]; then
    echo "Usage: $0 <p4_file>"
    echo "Example: $0 simple_routing.p4"
    exit 1
fi

P4_FILE=$1
PROGRAM_NAME=$(basename "$P4_FILE" .p4)

echo "Compiling P4 program: $P4_FILE"

# Check if p4c is installed
if ! command -v p4c &> /dev/null; then
    echo "Error: p4c compiler not found. Please install p4c."
    echo "Ubuntu/Debian: sudo apt-get install p4lang-p4c"
    exit 1
fi

# Create output directory
OUTPUT_DIR="compiled"
mkdir -p "$OUTPUT_DIR"

# Compile P4 program for BMv2
echo "Running p4c compiler..."
p4c \
    -b bmv2 \
    --p4runtime-files "$OUTPUT_DIR/$PROGRAM_NAME.p4info.txt" \
    --json "$OUTPUT_DIR/$PROGRAM_NAME.json" \
    "$P4_FILE"

if [ $? -ne 0 ]; then
    echo "Error: P4 compilation failed"
    exit 1
fi

echo ""
echo "Compilation successful!"
echo ""
echo "Output files:"
echo "  - $OUTPUT_DIR/$PROGRAM_NAME.p4info.txt  (P4Info text format - human readable)"
echo "  - $OUTPUT_DIR/$PROGRAM_NAME.json        (BMv2 device config)"
echo ""
echo "Next steps:"
echo "1. Copy the compiled files to your deployment location"
echo "2. Deploy via API: POST /api/p4/program with the program data"
