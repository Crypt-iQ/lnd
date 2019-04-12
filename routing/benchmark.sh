#!/bin/sh

for i in $(seq 1 500)
do
    GO111MODULE=on go test -run TestVisitedPathFind
    sleep 3s
done

