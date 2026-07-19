#!/bin/bash

# Remove the polkit rule installed by helm-cli setup-polkit; the package no
# longer manages these services, so the passwordless grant must go with it.
rm -f /etc/polkit-1/rules.d/99-helm.rules

exit 0
