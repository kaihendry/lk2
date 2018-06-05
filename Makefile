all: statik
	go get
	go build

statik: clean
	go generate

clean:
	rm -rf statik dist/ gin-bin DCIM lk2
