#!/bin/bash

cmd=~/tools/go_appengine/dev_appserver.py

$cmd \
    --storage_path=~/tmp/appengine/innerhearth/ \
    $PWD
