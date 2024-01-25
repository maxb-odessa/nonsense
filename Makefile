
PREFIX		=	${HOME}/.local
SHAREDIR	=	${PREFIX}/share/nonsense
CONFDIR		=	${PREFIX}/etc
GOBIN		=	${PREFIX}/bin

GO111MODULE	=	auto

all: build

build:
	go build ./cmd/nonsense

install:
	go env -w GOBIN=${GOBIN}
	go install ./cmd/nonsense
	mkdir -p ${SHAREDIR}
	cp -a res/* ${SHAREDIR}
	mkdir -p ${CONFDIR}
	cp -a conf/nonsense.conf ${CONFDIR}
	cp -a nonsense.service ${HOME}/.config/systemd/user/
	systemctl --user daemon-reload

