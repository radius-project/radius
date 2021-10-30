#! /bin/bash
set -u

ps | grep kcp | grep -v grep
if [[ ! $? -eq 0 ]]; then
    echo 'kcp must be started before running radiusd-run target ("~/.rad/bin/kcp start")'
    exit 1
fi

if [[ ! -e ~/.rad/bin/.kcp/data/admin.kubeconfig ]]; then
    echo 'kcp cubeconfig file not found (expected "~/.rad/bin/.kcp/data/admin.kubeconfig" to exist'
    exit 2
fi
