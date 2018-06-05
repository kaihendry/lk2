all: statik
	go build

statik: clean
	go generate

clean:
	rm -rf statik dist/ gin-bin DCIM lk2
