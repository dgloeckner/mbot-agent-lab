# Azure VM

Target setup: the server runs on an Azure VM, robots connect from the guest Wi-Fi via MQTT over
TLS (8883), the lab PCs in the corporate network only need HTTPS (443) in the browser.

1. Create a small Ubuntu VM (e.g. B2s) with a public IP and DNS names for `lab.` and `mqtt.`;
   pass `cloud-init.yaml` as custom data. Open ports 80, 443 and 8883 in the NSG.
2. Copy `configs/lab.example.yaml` to `/etc/labserver/lab.yaml` and adapt hosts, robots, teams.
3. Create `/etc/labserver/secrets.env` (mode 600) with all variables named in the config.
4. Install `deploy/systemd/labserver.service` to `/etc/systemd/system/` and enable it.
5. `make deploy HOST=user@vm` builds for linux/amd64, copies the binary and restarts the service.

Open points: VM creation script (`az` CLI), whether port 80 is needed for ACME, backups of
`/var/lib/labserver/lab.db`.
