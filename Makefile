# Makefile
# Assumes GOPATH is set up properly, e.g., $HOME/go

default:
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o build/macos/awsinfo

all:
	rm -rf build
	mkdir -p build/{macos,centos,windows}
	go get -u github.com/aws/aws-sdk-go/...
	go get -u github.com/vaughan0/go-ini
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o build/macos/awsinfo
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o build/centos/awsinfo
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o build/windows/awsinfo.exe

install:
	./install.sh build/macos/awsinfo

clean:
	rm -rf build
