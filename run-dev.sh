#!/bin/bash

cmd=~/tools/google_appengine/dev_appserver.py

$cmd \
    --storage_path=~/tmp/appengine/innerhearth/ \
    $PWD
