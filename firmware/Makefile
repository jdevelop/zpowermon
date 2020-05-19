.PHONY: zpowermon

PATCHDATE?=$(shell date +%s)

DOCKERREG?=jdevelop
VERSION?=latest

zpowermon:
	docker build -t ${DOCKERREG}/zpowermon:${VERSION} -f Dockerfile . --build-arg VER=`git rev-parse HEAD`
	docker push ${DOCKERREG}/zpowermon:${VERSION}
	kubectl apply -f kube/*
