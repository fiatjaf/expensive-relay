expensive-relay: $(shell find .. -name "*.go")
	go build -ldflags="-s -w" -o ./expensive-relay

deploy: expensive
	ssh root@turgot 'systemctl stop expensive-relay'
	scp expensive-relay turgot:relayer/relayer
	ssh root@turgot 'systemctl start expensive-relay'
