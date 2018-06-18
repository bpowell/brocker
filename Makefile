all:
	go install ./...

install:
	go install ./...
	sudo mkdir -p /etc/brocker
	sudo cp -R etc/brocker/ /etc/
