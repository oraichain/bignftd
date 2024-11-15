#!/bin/bash

BINARY=oraid
CHAIN_DIR=$PWD/data
CHAINID_1=test-1
CHAINID_2=test-2

VAL_MNEMONIC_1="clock post desk civil pottery foster expand merit dash seminar song memory figure uniform spice circle try happy obvious trash crime hybrid hood cushion"
VAL_MNEMONIC_2="angry twist harsh drastic left brass behave host shove marriage fall update business leg direct reward object ugly security warm tuna model broccoli choice"
WALLET_MNEMONIC_1="banner spread envelope side kite person disagree path silver will brother under couch edit food venture squirrel civil budget number acquire point work mass"
WALLET_MNEMONIC_2="veteran try aware erosion drink dance decade comic dawn museum release episode original list ability owner size tuition surface ceiling depth seminar capable only"
WALLET_MNEMONIC_3="vacuum burst ordinary enact leaf rabbit gather lend left chase park action dish danger green jeans lucky dish mesh language collect acquire waste load"
WALLET_MNEMONIC_4="open attitude harsh casino rent attitude midnight debris describe spare cancel crisp olive ride elite gallery leaf buffalo sheriff filter rotate path begin soldier"
RLY_MNEMONIC_1="alley afraid soup fall idea toss can goose become valve initial strong forward bright dish figure check leopard decide warfare hub unusual join cart"
RLY_MNEMONIC_2="record gift you once hip style during joke field prize dust unique length more pencil transfer quit train device arrive energy sort steak upset"

P2PPORT_1=16656
P2PPORT_2=26656
RPCPORT_1=16657
RPCPORT_2=26657
RESTPORT_1=1316
RESTPORT_2=1317
ROSETTA_1=9080
ROSETTA_2=9081

# Stop if it is already running 
if pgrep -x "$BINARY" >/dev/null; then
    echo "Terminating $BINARY..."
    killall $BINARY
fi

echo "Removing previous data..."
rm -rf $CHAIN_DIR/$CHAINID_1 &> /dev/null
rm -rf $CHAIN_DIR/$CHAINID_2 &> /dev/null

# Add directories for both chains, exit if an error occurs
if ! mkdir -p $CHAIN_DIR/$CHAINID_1 2>/dev/null; then
    echo "Failed to create chain folder. Aborting..."
    exit 1
fi

if ! mkdir -p $CHAIN_DIR/$CHAINID_2 2>/dev/null; then
    echo "Failed to create chain folder. Aborting..."
    exit 1
fi

echo "Initializing $CHAINID_1..."
echo "Initializing $CHAINID_2..."
$BINARY init test --home $CHAIN_DIR/$CHAINID_1 --chain-id=$CHAINID_1
$BINARY init test --home $CHAIN_DIR/$CHAINID_2 --chain-id=$CHAINID_2

# Update host chain genesis to allow x/bank/MsgSend ICA tx execution
cat $CHAIN_DIR/$CHAINID_2/config/genesis.json | jq '(.app_state.interchainaccounts.host_genesis_state.params.allow_messages) |= ["/cosmos.bank.v1beta1.MsgSend", "/cosmos.staking.v1beta1.MsgDelegate"]' > $CHAIN_DIR/$CHAINID_2/config/genesis.json.tmp && mv $CHAIN_DIR/$CHAINID_2/config/genesis.json.tmp $CHAIN_DIR/$CHAINID_2/config/genesis.json

update_genesis () {
    cat $2/config/genesis.json | jq "$1" > $2/config/tmp_genesis.json && mv $2/config/tmp_genesis.json $2/config/genesis.json
}

# update crisis variable to orai
update_genesis '.app_state["crisis"]["constant_fee"]["denom"]="orai"' $CHAIN_DIR/$CHAINID_2
update_genesis '.app_state["crisis"]["constant_fee"]["denom"]="orai"' $CHAIN_DIR/$CHAINID_1
# udpate gov genesis
update_genesis '.app_state["gov"]["deposit_params"]["min_deposit"][0]["denom"]="orai"' $CHAIN_DIR/$CHAINID_2
update_genesis '.app_state["gov"]["deposit_params"]["min_deposit"][0]["denom"]="orai"' $CHAIN_DIR/$CHAINID_1
update_genesis '.app_state["gov"]["voting_params"]["voting_period"]="6s"' $CHAIN_DIR/$CHAINID_2
update_genesis '.app_state["gov"]["voting_params"]["voting_period"]="6s"' $CHAIN_DIR/$CHAINID_1
# update mint genesis
update_genesis '.app_state["mint"]["params"]["mint_denom"]="orai"' $CHAIN_DIR/$CHAINID_2
update_genesis '.app_state["mint"]["params"]["mint_denom"]="orai"' $CHAIN_DIR/$CHAINID_1

echo "Adding genesis accounts..."
echo $VAL_MNEMONIC_1 | $BINARY keys add val1 --home $CHAIN_DIR/$CHAINID_1 --recover --keyring-backend=test
echo $VAL_MNEMONIC_2 | $BINARY keys add val2 --home $CHAIN_DIR/$CHAINID_2 --recover --keyring-backend=test
echo $WALLET_MNEMONIC_1 | $BINARY keys add wallet1 --home $CHAIN_DIR/$CHAINID_1 --recover --keyring-backend=test
echo $WALLET_MNEMONIC_2 | $BINARY keys add wallet2 --home $CHAIN_DIR/$CHAINID_1 --recover --keyring-backend=test
echo $WALLET_MNEMONIC_3 | $BINARY keys add wallet3 --home $CHAIN_DIR/$CHAINID_2 --recover --keyring-backend=test
echo $WALLET_MNEMONIC_4 | $BINARY keys add wallet4 --home $CHAIN_DIR/$CHAINID_2 --recover --keyring-backend=test
echo $RLY_MNEMONIC_1 | $BINARY keys add rly1 --home $CHAIN_DIR/$CHAINID_1 --recover --keyring-backend=test 
echo $RLY_MNEMONIC_2 | $BINARY keys add rly2 --home $CHAIN_DIR/$CHAINID_2 --recover --keyring-backend=test 

$BINARY add-genesis-account $($BINARY --home $CHAIN_DIR/$CHAINID_1 keys show val1 --keyring-backend test -a) 100000000000orai  --home $CHAIN_DIR/$CHAINID_1
$BINARY add-genesis-account $($BINARY --home $CHAIN_DIR/$CHAINID_2 keys show val2 --keyring-backend test -a) 100000000000orai  --home $CHAIN_DIR/$CHAINID_2
$BINARY add-genesis-account $($BINARY --home $CHAIN_DIR/$CHAINID_1 keys show wallet1 --keyring-backend test -a) 100000000000orai  --home $CHAIN_DIR/$CHAINID_1
$BINARY add-genesis-account $($BINARY --home $CHAIN_DIR/$CHAINID_1 keys show wallet2 --keyring-backend test -a) 100000000000orai  --home $CHAIN_DIR/$CHAINID_1
$BINARY add-genesis-account $($BINARY --home $CHAIN_DIR/$CHAINID_2 keys show wallet3 --keyring-backend test -a) 100000000000orai  --home $CHAIN_DIR/$CHAINID_2
$BINARY add-genesis-account $($BINARY --home $CHAIN_DIR/$CHAINID_2 keys show wallet4 --keyring-backend test -a) 100000000000orai  --home $CHAIN_DIR/$CHAINID_2
$BINARY add-genesis-account $($BINARY --home $CHAIN_DIR/$CHAINID_1 keys show rly1 --keyring-backend test -a) 100000000000orai  --home $CHAIN_DIR/$CHAINID_1
$BINARY add-genesis-account $($BINARY --home $CHAIN_DIR/$CHAINID_2 keys show rly2 --keyring-backend test -a) 100000000000orai  --home $CHAIN_DIR/$CHAINID_2

echo "Creating and collecting gentx..."
$BINARY gentx val1 7000000000orai --home $CHAIN_DIR/$CHAINID_1 --chain-id $CHAINID_1 --keyring-backend test
$BINARY gentx val2 7000000000orai --home $CHAIN_DIR/$CHAINID_2 --chain-id $CHAINID_2 --keyring-backend test
$BINARY collect-gentxs --home $CHAIN_DIR/$CHAINID_1
$BINARY collect-gentxs --home $CHAIN_DIR/$CHAINID_2

echo "Changing defaults and ports in app.toml and config.toml files..."
sed -i '' -E 's#"tcp://0.0.0.0:26656"#"tcp://0.0.0.0:'"$P2PPORT_1"'"#g' $CHAIN_DIR/$CHAINID_1/config/config.toml
sed -i '' -E 's#"tcp://127.0.0.1:26657"#"tcp://0.0.0.0:'"$RPCPORT_1"'"#g' $CHAIN_DIR/$CHAINID_1/config/config.toml
sed -i '' -E 's/timeout_commit = "5s"/timeout_commit = "1s"/g' $CHAIN_DIR/$CHAINID_1/config/config.toml
sed -i '' -E 's/timeout_propose = "3s"/timeout_propose = "1s"/g' $CHAIN_DIR/$CHAINID_1/config/config.toml
sed -i '' -E 's/index_all_keys = false/index_all_keys = true/g' $CHAIN_DIR/$CHAINID_1/config/config.toml
sed -i '' -E 's/enable = false/enable = true/g' $CHAIN_DIR/$CHAINID_1/config/app.toml
sed -i '' -E 's/swagger = false/swagger = true/g' $CHAIN_DIR/$CHAINID_1/config/app.toml
sed -i '' -E 's/1317/'"$RESTPORT_1"'/g' $CHAIN_DIR/$CHAINID_1/config/app.toml
sed -i '' -E 's/":8080"/":'"$ROSETTA_1"'"/g' $CHAIN_DIR/$CHAINID_1/config/app.toml
sed -i '' -E "s%^minimum-gas-prices *=.*%minimum-gas-prices = \"0orai\"%; " $CHAIN_DIR/$CHAINID_1/config/app.toml
sed -i '' -E 's|0.0.0.0:9090|0.0.0.0:8090|g' $CHAIN_DIR/$CHAINID_1/config/app.toml
sed -i '' -E 's|0.0.0.0:9091|0.0.0.0:8091|g' $CHAIN_DIR/$CHAINID_1/config/app.toml

sed -i '' -E 's#"tcp://0.0.0.0:26656"#"tcp://0.0.0.0:'"$P2PPORT_2"'"#g' $CHAIN_DIR/$CHAINID_2/config/config.toml
sed -i '' -E 's#"tcp://127.0.0.1:26657"#"tcp://0.0.0.0:'"$RPCPORT_2"'"#g' $CHAIN_DIR/$CHAINID_2/config/config.toml
sed -i '' -E 's/timeout_commit = "5s"/timeout_commit = "1s"/g' $CHAIN_DIR/$CHAINID_2/config/config.toml
sed -i '' -E 's/timeout_propose = "3s"/timeout_propose = "1s"/g' $CHAIN_DIR/$CHAINID_2/config/config.toml
sed -i '' -E 's/index_all_keys = false/index_all_keys = true/g' $CHAIN_DIR/$CHAINID_2/config/config.toml
sed -i '' -E 's/enable = false/enable = true/g' $CHAIN_DIR/$CHAINID_2/config/app.toml
sed -i '' -E 's/swagger = false/swagger = true/g' $CHAIN_DIR/$CHAINID_2/config/app.toml
sed -i '' -E 's/1317/'"$RESTPORT_2"'/g' $CHAIN_DIR/$CHAINID_2/config/app.toml
sed -i '' -E 's/":8080"/":'"$ROSETTA_2"'"/g' $CHAIN_DIR/$CHAINID_2/config/app.toml
# modify jsonrpc ports to avoid clashing
sed -i '' -E 's|0.0.0.0:8545|0.0.0.0:7545|g' $CHAIN_DIR/$CHAINID_2/config/app.toml
sed -i '' -E "s%^ws-address *=.*%ws-address = \"0.0.0.0:7546\"%; " $CHAIN_DIR/$CHAINID_2/config/app.toml
sed -i '' -E "s%^minimum-gas-prices *=.*%minimum-gas-prices = \"0orai\"%; " $CHAIN_DIR/$CHAINID_2/config/app.toml