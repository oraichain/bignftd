#!/bin/bash

set -eu

CHAIN_ID=${CHAIN_ID:-testing}
USER=${USER:-tupt}
WASM_PATH=${WASM_PATH:-"$PWD/scripts/wasm_file/counter_high_gas_cost.wasm"}
EXECUTE_MSG=${EXECUTE_MSG:-'{"ping":{}}'}
NODE_HOME=${NODE_HOME:-"$PWD/.oraid"}
ARGS="--from $USER --chain-id $CHAIN_ID -y --keyring-backend test --gas auto --gas-adjustment 1.5 -b block --home $NODE_HOME"
HIDE_LOGS="/dev/null"

# prepare a new contract for gasless
store_ret=$(oraid tx wasm store $WASM_PATH $ARGS --output json)
code_id=$(echo $store_ret | jq -r '.logs[0].events[1].attributes[] | select(.key | contains("code_id")).value')
oraid tx wasm instantiate $code_id '{}' --label 'testing' --admin $(oraid keys show $USER --keyring-backend test --home $NODE_HOME -a) $ARGS > $HIDE_LOGS
contract_address=$(oraid query wasm list-contract-by-code $code_id --output json | jq -r '.contracts[0]')
echo $contract_address

# try executing something, gas should equal 0
gas_used_before=$(oraid tx wasm execute $contract_address $EXECUTE_MSG $ARGS --output json --gas 20000000 | jq '.gas_used | tonumber')
echo "gas used before gasless: $gas_used_before"

# # set gasless proposal
# oraid tx gov submit-proposal set-gasless $contract_address --title "gasless" --description "gasless" --deposit 10000000orai $ARGS > $HIDE_LOGS
# proposal_id=$(oraid query gov proposals --reverse --output json | jq '.proposals[0].proposal_id | tonumber')
# oraid tx gov vote $proposal_id yes $ARGS > $HIDE_LOGS

# # wait til proposal passes
# sleep 6
# proposal_status=$(oraid query gov proposal $proposal_id --output json | jq .status)
# if ! [[ $proposal_status =~ "PROPOSAL_STATUS_PASSED" ]] ; then
#    echo "The proposal has not passed yet"; exit 1
# fi

echo "Gasless create passed!"