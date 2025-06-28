NAME=nfrouter
ARCH=$(shell uname -m)
SRC_NAME=$(NAME).c
OUT_NAME=$(NAME).o


build: $(OUT_NAME)
	clang -O2 -g -Wall -target bpf \
		-I/usr/include \
		-I/usr/include/$(ARCH)-linux-gnu \
		-c $(SRC_NAME) -o $(OUT_NAME)

clean:
	rm -rf $(OUT_NAME)

