GO ?= $(shell which go)
OS ?= $(shell $(GO) env GOOS)
ARCH ?= $(shell $(GO) env GOARCH)

IMAGE_TAG ?= "latest"
IMAGE_NAME := "cert-manager-webhook-luadns"

OUT := $(shell pwd)/_out

KUBEBUILDER_VERSION=1.30.0

HELM_FILES := $(shell find charts/cert-manager-webhook-luadns)

.PHONY: all
all: build

.PHONY: test
test: _test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/etcd _test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/kube-apiserver _test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/kubectl
	cd pkg && \
	TEST_ASSET_ETCD=../_test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/etcd \
	TEST_ASSET_KUBE_APISERVER=../_test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/kube-apiserver \
	TEST_ASSET_KUBECTL=../_test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/kubectl \
	$(GO) test -v .

_test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH).tar.gz: | _test
	curl -fsSL https://go.kubebuilder.io/test-tools/$(KUBEBUILDER_VERSION)/$(OS)/$(ARCH) -o $@

_test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/etcd _test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/kube-apiserver _test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)/kubectl: _test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH).tar.gz | _test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH)
	tar xfO $< kubebuilder/bin/$(notdir $@) > $@ && chmod +x $@

.PHONY: clean
clean:
	rm -Rf _test $(OUT)

.PHONY: build
build:
	buildah build -t "$(IMAGE_NAME):$(IMAGE_TAG)" .

.PHONY: rendered-manifest.yaml
rendered-manifest.yaml: $(OUT)/rendered-manifest.yaml

$(OUT)/rendered-manifest.yaml: $(HELM_FILES) | $(OUT)
	helm template \
	    --set image.repository=$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG) \
		charts/cert-manager-webhook-luadns > $@

_test $(OUT) _test/kubebuilder-$(KUBEBUILDER_VERSION)-$(OS)-$(ARCH):
	mkdir -p $@
