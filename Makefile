# Default installation directory.
#	make install dir=$HOME/bin
dir ?= /bin/
mdir ?= /usr/share/man/man1/
root ?= root
group ?= root

.PHONY: all
all: bin/ovh-do

.PHONY: help
help:
	@echo 'all'
	@echo '            build bin/ovh-do'
	@echo 'clean'
	@echo '            removed compiled files'
	@echo 'install dir=... mdir=... group=... root=...'
	@echo '            install bin/* to $dir (default /bin/);'
	@echo '            man pages to $mdir (default /usr/share/man/man1/);'
	@echo '            default owner:group is root:root'
	@echo 'uninstall dir=...'
	@echo '            uninstall bin/* from $dir (default: /bin/)'

bin/ovh-do: ovh-do.go
	@echo Compiling ovh-do...
	@go build -o $@ $^

.PHONY: clean
clean:
	@echo Remove compiled binaries...
	@rm -f bin/ovh-do

.PHONY: install
install: bin/ovh-do ovh-do.1
	@echo "Installing bin/* to ${dir}/..."
	@for x in bin/*; do \
		install -o ${root} -g ${group} -m 755 $$x ${dir}/`basename $$x`; \
	done
	@echo "Installing ovh-do.1..."
	@install -o ${root} -g ${group} -m 644 ovh-do.1 ${mdir}/ovh-do.1

.PHONY: uninstall
uninstall:
	@echo "Removing all bin/* from ${dir}/..."
	@for x in bin/ovh-do; do echo rm -f ${dir}/`basename $$x`; done
