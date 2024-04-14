# Orbit Play

<h1 align="center">
  <img src="logo.png" alt="Orbit Play">
  <br>
</h1>

_Orbit Play_ is a media server that allows publishing, reading, and recording video and audio streams.

## Installation

There are several installation methods available: standalone binary, Docker image.

### Standalone Binary

1. **Create a Directory and Download Binary:**
    ```sh 
    mkdir media
    cd media
    ```
   Download and extract the standalone binary from the [drive link](https://drive.google.com/drive/folders/18fdmQQA1C2TGCqx9ysinnRJn-3foRuSu?usp=drive_link).

2. **Generate Required Certificates:**
    ```sh
    sudo apt update
    sudo apt install certbot python3-certbot-nginx
    sudo systemctl stop nginx
    sudo certbot certonly --standalone -d media.sstlive.com
    sudo cp /etc/letsencrypt/live/s2tlive.com/privkey.pem server.key
    sudo cp /etc/letsencrypt/live/s2tlive.com/fullchain.pem server.crt
    sudo chown ubuntu:ubuntu server.key
    sudo chown ubuntu:ubuntu server.crt
    ```

3. **Download and Set Permissions for Binaries:**
    ```sh
    wget --no-check-certificate 'https://drive.google.com/uc?export=download&id=1eQ6XrK0ovxUQw1XnYV363pfAv6JO_4uy' -O orbit-play
    chmod +x orbit-play

    wget --no-check-certificate 'https://drive.google.com/uc?export=download&id=12a1lcEil3l7vdGYwOo40-J1PP2XMb82G' -O orbit-play-wasabi-sync
    chmod +x orbit-play-wasabi-sync

    wget --no-check-certificate 'https://drive.google.com/uc?export=download&id=1RAiX9T6EJdpohr5_K6pALPt1-VP7_Edv' -O orbit-play.yml
    ```

4. **Create a Systemd Service File:**
   ```
    sudo cp orbit-launch.service /etc/systemd/system/
    ```
   
    ```sh
    sudo nano /etc/systemd/system/orbit-play.service
    ```

   Paste the following content:

    ```sh
    [Unit]
    Description=Orbit Play Service
    After=network.target

    [Service]
    ExecStart=/home/ubuntu/Media/orbit-play
    WorkingDirectory=/home/ubuntu/Media
    User=root
    Restart=always

    [Install]
    WantedBy=multi-user.target
    ```

5. **Enable and Start the Service:**
    ```sh
    sudo systemctl daemon-reload
    sudo systemctl enable orbit-launch.service 
    sudo systemctl start orbit-launch.service
    ```

6. **Start the Wasabi Server:**
    ```sh
    ./orbit-play-wasabi-sync
    ```

7. **Start the Media Server:**
    ```sh
    ./orbit-play
    ```
