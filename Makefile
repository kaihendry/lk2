statik: clean
	go generate

clean:
	rm -rf statik dist/ gin-bin DCIM
