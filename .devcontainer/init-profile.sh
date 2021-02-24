#!/bin/bash -i
set -eu

USERNAME=$1

echo "running init-profile.sh"
echo "current user is $(whoami)"
echo "target user is $(whoami)"
echo "current user HOME is $HOME"
echo "target user HOME is /home/$USERNAME/"

if [[ -e "/home/$USERNAME/.profile" ]] 
then
    echo "/home/$USERNAME/.profile exists"
else 
    echo "/home/$USERNAME/.profile not found"
fi

if [[ -e "/home/$USERNAME/.bashrc" ]]
then
    echo "/home/$USERNAME/.bashrc exists"
else 
    echo "/home/$USERNAME/.bashrc not found"
fi

if [[ -e "/home/$USERNAME/.zshrc" ]] 
then
    echo "/home/$USERNAME/.zshrc exists"
else 
    echo "/home/$USERNAME/.zshrc not found"
fi

chown $USERNAME:root /usr/local/share/copy-kube-config.sh \
    && echo "source /usr/local/share/copy-kube-config.sh" | tee -a /root/.bashrc /root/.zshrc /home/$USERNAME/.bashrc >> /home/$USERNAME/.zshrc