sudo cp jetson_stats_prometheus_collector.py /usr/local/bin/

# Install service for the current user

sudo cp jetson_stats_prometheus_collector.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl start jetson_stats_prometheus_collector
sudo systemctl status jetson_stats_prometheus_collector
sudo systemctl enable jetson_stats_prometheus_collector