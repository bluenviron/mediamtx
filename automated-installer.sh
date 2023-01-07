#!/usr/bin/env bash

# Require script to be run as root
function super-user-check() {
  # This code checks to see if the script is running with root privileges.
  # If it is not, it will exit with an error message.
  if [ "${EUID}" -ne 0 ]; then
    echo "Error: You need to run this script as administrator."
    exit
  fi
}

# Check for root
super-user-check
