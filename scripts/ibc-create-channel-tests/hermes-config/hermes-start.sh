#!/bin/bash

set -ux

# Configure predefined mnemonic pharses
BINARY=hermes
CHAIN_DIR=$PWD/data
CHAINID_1=test-1
CHAINID_2=test-2
MNEMONIC_1="alley afraid soup fall idea toss can goose become valve initial strong forward bright dish figure check leopard decide warfare hub unusual join cart"
MNEMONIC_2="record gift you once hip style during joke field prize dust unique length more pencil transfer quit train device arrive energy sort steak upset"

# Ensure rly is installed
if ! [ -x "$(command -v $BINARY)" ]; then
    echo "$BINARY is required to run this script..."
    exit 1
fi

echo "Starting to listen relayer..."
$BINARY start