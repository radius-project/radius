#!/bin/bash
# Simple script to read PID from file
# Usage: get-pid.sh <component-name>

if [ -z "$1" ]; then
  echo "0"
  exit 1
fi

PID_FILE="debug_files/logs/$1.pid"

if [ -f "$PID_FILE" ]; then
  cat "$PID_FILE"
else
  echo "0"
fi
