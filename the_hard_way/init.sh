#!/usr/bin/env bash

echo ">>> Starting initial configuration <<<"

echo "[1/7] Configuring shell environment"
echo 'alias vi=vim' >> /etc/profile
echo 'export HISTTIMEFORMAT="%F %T "' >> /etc/profile
ln -sf /usr/share/zoneinfo/UTC /etc/localtime

echo "[2/7] Disabling AppArmor"
systemctl stop apparmor 2>/dev/null
systemctl disable apparmor 2>/dev/null

echo "[3/7] Disabling swap"
swapoff -a
sed -i '/swap/s/^/#/' /etc/fstab

echo "[4/7] Installing required packages"
apt-get update -qq
apt-get install -y -qq tree git jq curl wget vim sshpass net-tools dnsutils > /dev/null 2>&1

echo "[5/7] Setting root password"
echo "root:kubernetes" | chpasswd

echo "[6/7] Configuring SSH"
sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config
sed -i 's/^#*PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config
systemctl restart sshd

echo "[7/7] Configuring /etc/hosts"
cat >> /etc/hosts << HOSTS
192.168.10.10  jumpbox
192.168.10.100 server server.kubernetes.local
192.168.10.101 node-0 node-0.kubernetes.local
192.168.10.102 node-1 node-1.kubernetes.local
HOSTS

echo ">>> Initial configuration complete <<<"
