#!/usr/bin/env bats
load $BATS_TEST_DIRNAME/helper/common.bash

setup() {
    skip_nbf_dolt
    skip_nbf_dolt_dev

    setup_common
    TARGET_NBF="__DOLT__"
}

teardown() {
    teardown_common
}

@test "migration-integration: first-hour-db" {
    dolt clone dolthub/first-hour-db
    cd first-hour-db

    dolt tag -v
    run dolt tag -v
    [ "$status" -eq 0 ]
    [[ "$output" =~ "r9jv07tf9un3fm1fg72v7ad9er89oeo7" ]] || false
    [[ ! "$output" =~ "eu5pgaa5kgsapqts6sar19isnegmqpqn" ]] || false

    dolt migrate
    [[ $(cat ./.dolt/noms/manifest | cut -f 2 -d :) = "$TARGET_NBF" ]] || false

    dolt tag -v
    run dolt tag -v
    [ "$status" -eq 0 ]
    [[ "$output" =~ "eu5pgaa5kgsapqts6sar19isnegmqpqn" ]] || false
    [[ ! "$output" =~ "r9jv07tf9un3fm1fg72v7ad9er89oeo7" ]] || false

    # validate TEXT migration
    run dolt sql -q "select film_id, title from film order by film_id limit 1" -r csv
    [ "$status" -eq 0 ]
    [[ "$output" =~ "1,ACADEMY DINOSAUR" ]] || false
}

@test "migration-integration: us-jails" {
    dolt clone dolthub/us-jails
    cd us-jails

    dolt tag -v
    run dolt tag -v
    [ "$status" -eq 0 ]
    [[ "$output" =~ "u8s83gapv7ghnbmrtpm8q5es0dbl7lpd" ]] || false
    [[ ! "$output" =~ "seaiu0pkcldfkjbv5ejb8e1ravb74c4k" ]] || false

    dolt migrate
    [[ $(cat ./.dolt/noms/manifest | cut -f 2 -d :) = "$TARGET_NBF" ]] || false

    dolt tag -v
    run dolt tag -v
    [ "$status" -eq 0 ]
    [[ "$output" =~ "seaiu0pkcldfkjbv5ejb8e1ravb74c4k" ]] || false
    [[ ! "$output" =~ "u8s83gapv7ghnbmrtpm8q5es0dbl7lpd" ]] || false
}
