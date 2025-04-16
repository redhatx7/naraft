#!/bin/bash

pkill -f "./raftx" 2>/dev/null

rm -rf ./logs
mkdir ./logs

sleep 2
echo "Starting Cluster"

go build -o raftx .

./raftx --port 50000 --id 1 --peers 2:localhost:50001,3:localhost:50002 > logs/node1.log 2>&1 &
echo "Started node 1"

sleep 2

./raftx --port 50001 --id 2 --peers 1:localhost:50000,3:localhost:50002 > logs/node2.log 2>&1 &
echo "Started node 2"
sleep 2

./raftx --port 50002 --id 3 --peers 1:localhost:50000,2:localhost:50001 > logs/node3.log 2>&1 &
echo "Started node 3"
sleep 2

# Tail logs
sleep 1
echo "Tailing logs..."
tail -f logs/node1.log logs/node2.log logs/node3.log