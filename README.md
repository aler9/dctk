
# dctoolkit

[![GoDoc](https://godoc.org/github.com/gswly/dctoolkit?status.svg)](https://godoc.org/github.com/gswly/dctoolkit)
[![Go Report Card](https://goreportcard.com/badge/github.com/gswly/dctoolkit)](https://goreportcard.com/report/github.com/gswly/dctoolkit)

dctoolkit implements the client part of the Direct Connect peer-to-peer system (ADC and NMDC protocols) in the Go programming language. It includes:
* a [**library**](#library), that allows the creation of clients capable of interacting with hubs and other clients;
* a series of [**command line utilities**](#command-line-utilities) that make use of the library.

Direct Connect is semi-centralized peer-to-peer system in which peers connect to servers (hubs) and exchange textual messages and files. Files are indexed by computing their Tiger Tree Hash (TTH), provided by users through their file list, and searchable on a hub-basis. There exist two variants, one based on the traditional NMDC protocol (NeoModus Direct Connect) and the other based on the newer ADC protocol (Advanced Direct Connect).

This project is based on the [**go-dc**](https://github.com/direct-connect/go-dc) project, that provides a base layer for building DC-related software.

## Features

* ADC and NMDC transparent protocol support
* **Active** and **passive** mode
* **Hub**: connection with configurable try count, password authentication, keepalive, compression, encryption
* **Chat**: bidirectional public and private chat
* **File search**: by name or TTH, reply to requests
* **File download**: by name or TTH, full or partial, on ram or disk, multiple in parallel, compression, encryption, configurable download slots, validation via TTH, client fingerprint validation
* **File upload**: upload from personal share, asynchronous file indexing system, file list generation and serving, compression, encryption, configurable upload slots, tthl extension support, client fingerprint validation
* Examples provided for every feature
* Comprehensive test suite

Note: this project uses the rolling release development model, as it is used in a production environment which requires the latest updates. The public API may suffer minor changes. The master branch is to be considered stable.

## Library

#### Installation

Go &ge; 1.11 is required. If modules are enabled (i.e. there's a go.mod file in your project folder), it is enough to write the library name in the import section of the source files that are referring to it. Go will take care of downloading the needed files:
```go
import (
    dctk "github.com/gswly/dctoolkit"
)
```

If modules are not enabled, the library must be downloaded manually:
```
go get github.com/gswly/dctoolkit
```

#### Examples

* [connection_active](example/1connection_active.go)
* [connection_passive](example/2connection_passive.go)
* [chat_public](example/3chat_public.go)
* [chat_private](example/4chat_private.go)
* [search](example/5search.go)
* [share](example/6share.go)
* [magnet](example/7magnet.go)
* [download_list](example/8download_list.go)
* [download_all_lists](example/9download_all_lists.go)
* [download_file](example/10download_file.go)
* [download_file_on_disk](example/11download_file_on_disk.go)
* [download_file_from_search](example/12download_file_from_search.go)
* [download_file_from_list](example/13download_file_from_list.go)
* [download_directory_from_list](example/14download_directory_from_list.go)

#### Documentation

https://godoc.org/github.com/gswly/dctoolkit

#### Testing

If you want to edit this library and test the results, you can you automated tests through:
```
make test
```

## Command-line utilities

#### Installation

Go &ge; 1.11 is required. Download, compile and install the binaries with a single command:
```
go get github.com/gswly/dctoolkit/cmd/...
```

#### Usage

```
dc-tth [<flags>] <filepath>

Compute the Tiger Tree Hash (TTH) of a given file.
```

```
dc-search --hub=HUB --nick=NICK [<flags>] <query>

Search files and directories by name in a given hub.
```

```
dc-download --hub=HUB --nick=NICK --outdir=OUTDIR [<flags>] <user> <fpath>

Download a file or a directory from a user in a given hub.
```

```
dc-share --hub=HUB --nick=NICK [<flags>] <share>

Share a directory in a given hub.
```

## Links

Protocol references
* (ADC) https://adc.dcbase.org/Protocol
* (ADC) https://adc.dcbase.org/Extensions
* (NMDC) http://nmdc.sourceforge.net/Versions/NMDC-1.3.html
* (NMDC) https://web.archive.org/web/20160412113951/http://wiki.gusari.org/index.php?title=Main_Page

Hubs
* (NMDC) http://www.ptokax.org
* (NMDC) https://github.com/Verlihub/verlihub
* (ADC) http://adchpp.sourceforge.net/
* (ADC) https://luadch.github.io/
* (ADC) https://www.uhub.org/
* (NMDC & ADC) https://sourceforge.net/projects/flexhubdc/
* (NMDC & ADC) http://rushub.org/
* (NMDC & ADC) https://github.com/direct-connect/go-dcpp

Clients
* (NMDC) https://dev.yorhel.nl/ncdc
* (NMDC & ADC) https://github.com/eiskaltdcpp/eiskaltdcpp
* (NMDC & ADC) http://dcplusplus.sourceforge.net/
* (NMDC & ADC) https://www.airdcpp.net/
* (NMDC & ADC) https://www.apexdc.net/
* (NMDC) http://jucy.eu/
* (NMDC & ADC) https://launchpad.net/linuxdcpp
* (NMDC) https://github.com/lilyball/dcbot

Other libraries
* [Go] (NMDC & ADC) https://git.ivysaur.me/code.ivysaur.me/libnmdc
* [Go] (ADC) https://github.com/ehmry/go-adc
* [Go] (ADC) https://github.com/seoester/adcl
* [Python] (NMDC) https://github.com/ashishgaurav13/nmdc
* [Python] (NMDC) http://pydc.sourceforge.net/
* [Python] (ADC) https://pypi.org/project/pyADC/

Inspired by
* https://godoc.org/github.com/anacrolix/torrent
