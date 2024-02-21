
PREFIX		=	${HOME}/.local
SHAREDIR	=	${PREFIX}/share/nonsens
CONFDIR		=	${PREFIX}/etc
GOBIN		=	${PREFIX}/bin

GO111MODULE	=	auto

all: build

build:
	go build ./cmd/nonsens

install:
	go env -w GOBIN=${GOBIN}
	go install ./cmd/nonsens
	mkdir -p ${SHAREDIR}
	cp -a res/* ${SHAREDIR}
	mkdir -p ${CONFDIR}
	cp -a etc/nonsens.json ${CONFDIR}
	cp -a nonsens.service ${HOME}/.config/systemd/user/
	systemctl --user daemon-reload
	systemctl --user enable nonsens
	systemctl --user restart nonsens

