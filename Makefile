LIBMODERNIMAGE_VERSION ?= 0.2.2

.PHONY: setup test-go test-typescript test-rust test-all clean release

setup:
	./scripts/setup.sh $(LIBMODERNIMAGE_VERSION)

test-go:
	cd golang && go test -v -timeout 120s

test-typescript:
	cd typescript && npm install && npm test

test-rust:
	cd rust && cargo test

test-all: test-go test-typescript test-rust

release:
	./scripts/release.sh

clean:
	rm -rf golang/shared/lib/*/libmodernimage.a
	rm -f golang/shared/include/modernimage.h
	rm -rf typescript/lib/*/libmodernimage.*
	rm -rf rust/lib/*/libmodernimage.a
	rm -f rust/modernimage.h
