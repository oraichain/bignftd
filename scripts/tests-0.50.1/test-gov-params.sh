#!/bin/bash
# Before running this script, you must setup local network:

set -e

burn_vote_veto=$(oraid query gov params --output json | jq -r '.params.burn_vote_veto')
expedited_min_deposit_amount=$(oraid query gov params --output json | jq -r '.params.expedited_min_deposit[0].amount | tonumber')
expedited_min_deposit_denom=$(oraid query gov params --output json | jq -r '.params.expedited_min_deposit[0].denom')
expedited_threshold=$(oraid query gov params --output json | jq -r '.params.expedited_threshold | tonumber')
expedited_voting_period=$(oraid query gov params --output json | jq -r '.params.expedited_voting_period')
max_deposit_period=$(oraid query gov params --output json | jq -r '.params.max_deposit_period')
min_deposit_amount=$(oraid query gov params --output json | jq -r '.params.min_deposit[0].amount | tonumber')
min_deposit_denom=$(oraid query gov params --output json | jq -r '.params.min_deposit[0].denom')
min_deposit_ratio=$(oraid query gov params --output json | jq -r '.params.min_deposit_ratio | tonumber')
min_initial_deposit_ratio=$(oraid query gov params --output json | jq -r '.params.min_initial_deposit_ratio')
proposal_cancel_ratio=$(oraid query gov params --output json | jq -r '.params.proposal_cancel_ratio | tonumber')
quorum=$(oraid query gov params --output json | jq -r '.params.quorum | tonumber')
threshold=$(oraid query gov params --output json | jq -r '.params.threshold | tonumber')
veto_threshold=$(oraid query gov params --output json | jq -r '.params.veto_threshold | tonumber')
voting_period=$(oraid query gov params --output json | jq -r '.params.voting_period')

# Define expected values
expected_burn_vote_veto="true"
expected_expedited_min_deposit_amount="50000000"
expected_expedited_min_deposit_denom="orai"
expected_expedited_threshold="0.667000000000000000"
expected_min_deposit_amount="10000000"
expected_min_deposit_denom="orai"
expected_min_deposit_ratio="0.010000000000000000"
expected_min_initial_deposit_ratio="0.000000000000000000"
expected_proposal_cancel_ratio="0.500000000000000000"
expected_quorum="0.334000000000000000"
expected_threshold="0.500000000000000000"
expected_veto_threshold="0.334000000000000000"

# Comparison without quotes on right-hand side
if [[ $burn_vote_veto != $expected_burn_vote_veto ]]; then
  echo "gov params Upgrade Failed: burn_vote_veto mismatch" >&2
  exit 1
fi

if [[ $expedited_min_deposit_amount -ne $expected_expedited_min_deposit_amount ]]; then
  echo "gov params Upgrade Failed: expedited_min_deposit_amount mismatch" >&2
  exit 1
fi

if [[ $expedited_min_deposit_denom != $expected_expedited_min_deposit_denom ]]; then
  echo "gov params Upgrade Failed: expedited_min_deposit_denom mismatch" >&2
  exit 1
fi

if [[ $expedited_threshold != $expected_expedited_threshold ]]; then
  echo "gov params Upgrade Failed: expedited_threshold mismatch" >&2
  exit 1
fi

if [[ $min_deposit_amount != $expected_min_deposit_amount ]]; then
  echo "gov params Upgrade Failed: min_deposit_amount mismatch" >&2
  exit 1
fi

if [[ $min_deposit_denom != $expected_min_deposit_denom ]]; then
  echo "gov params Upgrade Failed: min_deposit_denom mismatch" >&2
  exit 1
fi

if [[ $min_deposit_ratio != $expected_min_deposit_ratio ]]; then
  echo "gov params Upgrade Failed: min_deposit_ratio mismatch" >&2
  exit 1
fi

if [[ $min_initial_deposit_ratio != $expected_min_initial_deposit_ratio ]]; then
  echo "gov params Upgrade Failed: min_initial_deposit_ratio mismatch" >&2
  exit 1
fi

if [[ $proposal_cancel_ratio != $expected_proposal_cancel_ratio ]]; then
  echo "gov params Upgrade Failed: proposal_cancel_ratio mismatch" >&2
  exit 1
fi

if [[ $quorum != $expected_quorum ]]; then
  echo "gov params Upgrade Failed: quorum mismatch" >&2
  exit 1
fi

if [[ $threshold != $expected_threshold ]]; then
  echo "gov params Upgrade Failed: threshold mismatch" >&2
  exit 1
fi

if [[ $veto_threshold != $expected_veto_threshold ]]; then
  echo "gov params Upgrade Failed: veto_threshold mismatch" >&2
  exit 1
fi

echo "gov params Upgrade Succeeded"
