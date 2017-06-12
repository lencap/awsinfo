# Makefile
# Assumes GOPATH is set up in your system, e.g., $HOME/go

export TARGET := awsinfo

# Change this to the directory where you keep your binaries
MYBINDIR := $(HOME)/data/bin

default:
	go build -ldflags "-s -w" -o $(TARGET)
all:
	make clean
	go get -u github.com/aws/aws-sdk-go/...
	go get -u github.com/vaughan0/go-ini
	go build -ldflags "-s -w" -o $(TARGET)
install:
	cp $(TARGET) $(MYBINDIR)/
clean:
	rm -rf $(TARGET)
