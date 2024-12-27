go build -o ./tmp/main ./cmd/server
sudo -S setcap cap_net_raw,cap_net_admin=eip ./tmp/main
sudo chmod 777 ./tmp
