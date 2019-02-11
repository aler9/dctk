#!/bin/sh

run_test() {
    echo "[$PROTO $1]"

    if [ "$PROTO" = "nmdc" ]; then
        HUBIMAGE="dctk-verlihub"
        HUBURL="nmdc://dctk-hub:4111"
    else
        HUBIMAGE="dctk-luadch"
        HUBURL="adc://dctk-hub:5000"
    fi

    docker run --rm -d --network=dctk-test --name=dctk-hub \
        $HUBIMAGE $1 >/dev/null \
        || exit 1

    docker run --rm -it --network=dctk-test --name=dctk-test \
        -v ${PWD}:/src -e HUBURL=$HUBURL -e TEST=$1 dctk-test > $DEVOUT
    RETCODE=$?

    [ "$RETCODE" -eq 0 ] && echo "SUCCESS" || echo "FAILED"

    docker container kill dctk-hub >/dev/null 2>&1
}

usage() {
    echo "usage: $0 [--all] [-v] [nmdc|adc] [test_name]" 1>&2
    exit 1;
}

main() {
    # read arguments
    VERBOSE=0
    ALL=0
    PROTO=""
    TEST=""
    while [ $# -gt 0 ]; do
        case $1 in
            --all) ALL=1;;
            -v) VERBOSE=1;;
            -*) usage;;
            *) [ "$PROTO" = "" ] && PROTO=$1 || TEST=$1;;
        esac
        shift
    done

    if [ $ALL -eq 0 ]; then
        [ "$PROTO" != "nmdc" ] && [ "$PROTO" != "adc" ] && usage
        [ -n "$TEST" ] && [ -f "test/$TEST.go" ] || { echo "test not found"; exit 1; }
    fi

    [ $VERBOSE -eq 1 ] && DEVOUT=/dev/stdout || DEVOUT=/dev/null

    # cleanup residuals of previous tests
    docker container kill dctk-hub dctk-test >/dev/null 2>&1; \
        docker network rm dctk-test >/dev/null 2>&1

    # create images and network
    printf "building images..."
    ( docker build test/verlihub -t dctk-verlihub \
        && docker build test/luadch -t dctk-luadch \
        && docker build . -f test/Dockerfile -t dctk-test \
        && docker network create dctk-test ) > $DEVOUT \
        || exit 1
    echo "ok\n"

    # process
    if [ $ALL -eq 1 ]; then
        for PROTO in nmdc adc; do
            for TFILE in $(ls -v test/*.go); do
                run_test $(basename $TFILE | sed 's/\.go$//')
            done
        done
    else
        run_test $TEST
    fi

    # cleanup
    docker network rm dctk-test >/dev/null 2>&1
}

main $@
