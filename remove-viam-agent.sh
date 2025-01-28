#!/bin/bash

sudo systemctl stop viam-agent
sudo systemctl disable viam-agent
sudo rm /usr/local/lib/systemd/system/viam-agent.service
sudo systemctl daemon-reload
sudo systemctl reset-failed
sudo rm /etc/viam.json