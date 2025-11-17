#cloud-config
users:
  - name: ${username}
    groups: sudo
    shell: /bin/bash
    sudo: ["ALL=(ALL) NOPASSWD:ALL"]

runcmd:
  - printf "%s\n" "${motd}" > /etc/motd


