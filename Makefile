fmt:
	find . -name '*.go' -exec gofumpt -s -w -extra {} \;