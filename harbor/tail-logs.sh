#!/bin/bash
# Tail gollem agent logs from running Terminal-Bench containers.
# Usage: ./tail-logs.sh [container-name-pattern]
#   No args: shows all running containers and lets you pick
#   With pattern: tails matching container(s)

if [ -n "$1" ]; then
  # Tail specific container(s) matching the pattern
  for container in $(docker ps --format '{{.Names}}' | grep "$1"); do
    echo "=== $container ==="
    docker exec "$container" tail -f /tmp/gollem.log 2>/dev/null &
  done
  wait
else
  # List running containers
  echo "Running eval containers:"
  echo
  docker ps --format 'table {{.Names}}\t{{.Status}}' | grep -v "^NAMES" | while read line; do
    echo "  $line"
  done
  echo
  echo "Usage: $0 <pattern>"
  echo "  e.g., $0 gpt2       # tail gpt2-codegolf container"
  echo "  e.g., $0 batching   # tail batching-scheduler container"
  echo "  e.g., $0 main       # tail all containers"
fi
