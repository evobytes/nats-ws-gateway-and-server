#!/bin/sh

# Exit immediately if a command exits with a non-zero status
set -e

# Add a system user and group for the service if they don't already exist.
# The `getent` command is a robust way to check for existence across different Linux distros.
if ! getent group nats > /dev/null; then
    groupadd --system nats
fi

if ! getent passwd nats > /dev/null; then
    useradd \
        --system \
        --no-create-home \
        --shell /usr/sbin/nologin \
        -g nats \
        nats
fi

# Reload systemd, enable, and start the service
# The service file is installed by the package, but systemd needs to be
# reloaded to see the new unit file.
systemctl daemon-reload
systemctl enable nats-ws-gateway-and-server.service
systemctl start nats-ws-gateway-and-server.service

exit 0
