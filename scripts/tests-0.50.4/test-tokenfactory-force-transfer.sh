#!/bin/bash

set -ux

CHAIN_ID=${CHAIN_ID:-testing}
USER=${USER:-"validator1"}
NODE_HOME=${NODE_HOME:-"$PWD/.oraid"}
ARGS="--from $USER --chain-id $CHAIN_ID -y --keyring-backend test --gas auto --gas-adjustment 1.5 -b sync --home $NODE_HOME"
HIDE_LOGS="/dev/null"

# prepare a new contract for gasless
fee_params=$(oraid query tokenfactory params --output json | jq '.params.denom_creation_fee[0].denom')
if ! [[ $fee_params =~ "orai" ]]; then
   echo "Tokenfactory force transfer tests failed. The tokenfactory fee params is not orai"
   exit 1
fi

# try creating a new denom
denom_name="usd"
oraid tx tokenfactory create-denom $denom_name $ARGS >$HIDE_LOGS

# try querying list denoms afterwards
# need to sleep 1s
sleep 1
user_address=$(oraid keys show $USER --home $NODE_HOME --keyring-backend test -a)
first_denom=$(oraid query tokenfactory denoms-from-creator $user_address --output json | jq '.denoms[0]' | tr -d '"')
echo "first denom: $first_denom"

if ! [[ $first_denom =~ "factory/$user_address/$denom_name" ]]; then
   echo "Tokenfactory force transfer tests failed. The tokenfactory denom does not match the created denom"
   exit 1
fi

admin=$(oraid query tokenfactory denom-authority-metadata $first_denom --output json | jq '.authority_metadata.admin')
echo "admin: $admin"

if ! [[ $admin =~ $user_address ]]; then
   echo "Tokenfactory force transfer tests failed. The tokenfactory admin does not match the creator"
   exit 1
fi

sleep 2
# try to mint token 
oraid tx tokenfactory mint 10000$first_denom $ARGS >$HIDE_LOGS

# query balance after mint
# need sleep 1s
sleep 2
tokenfactory_balance=$(oraid query bank balance $user_address $first_denom --output json | jq '.balance.amount | tonumber')
if [[ $tokenfactory_balance -ne 10000 ]]; then
   echo "Tokenfactory force transfer failed. The tokenfactory balance does not increase after mint"
   exit 1
fi

# try to force transfer token to another address
oraid tx tokenfactory force-transfer 10$first_denom $user_address orai1cknd27x0244595pp7a5c9sdekl3ywl52x62ssn $ARGS &>$HIDE_LOGS

# query balance after force trasnfer
# need sleep 2s
sleep 2
tokenfactory_balance=$(oraid query bank balance $user_address $first_denom --output json | jq '.balance.amount | tonumber')
if ! [[ $tokenfactory_balance =~ 10000 ]]; then
   echo "Tokenfactory force transfer failed. The tokenfactory balance decreases after force transfer"
   exit 1
fi

echo "Tokenfactory force transfer tests passed!"
