# Installing PurplePages on Ubuntu

## 1. Create user and directory

```bash
sudo useradd -r -m -d /home/purplepages -s /bin/false purplepages
```

## 2. Build and deploy

```bash
# Build the binary (on your dev machine or the server)
go build -o purplepages .

# Copy files to the server
scp purplepages config.json user@server:/home/purplepages/

# On the server, set ownership
sudo chown -R purplepages:purplepages /home/purplepages
sudo chmod +x /home/purplepages/purplepages

# Create data directory
sudo -u purplepages mkdir -p /home/purplepages/data
```

## 3. Install systemd service

```bash
sudo cp purplepages.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable purplepages
sudo systemctl start purplepages
```

## 4. Check status

```bash
sudo systemctl status purplepages
sudo journalctl -u purplepages -f
```

## 5. Updating

```bash
sudo systemctl stop purplepages
# Copy new binary
sudo cp purplepages /home/purplepages/
sudo chown purplepages:purplepages /home/purplepages/purplepages
sudo systemctl start purplepages
```

## Nginx reverse proxy (optional)

```nginx
server {
    listen 443 ssl http2;
    server_name purplepag.es;

    ssl_certificate /etc/letsencrypt/live/purplepag.es/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/purplepag.es/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:3335;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }
}
```
