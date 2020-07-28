#!/bin/sh -e

exec ffmpeg -hide_banner -loglevel error $@
