
# dctoolkit

[![GoDoc](https://godoc.org/github.com/gswly/dctoolkit?status.svg)](https://godoc.org/github.com/gswly/dctoolkit)
[![Tag](https://img.shields.io/github/tag/gswly/dctoolkit.svg)](https://github.com/gswly/dctoolkit/tags)

dctoolkit is a project that implements the client part of the Direct Connect peer-to-peer system (ADC and NMDC protocols) in the Go programming language. It includes:
* a **library**, that allows the creation of clients capable of interacting with hubs and other clients;
* a series of **command line utilities** that make use of the library.

Direct Connect is semi-centralized peer-to-peer system in which peers connect to servers (hubs) and exchange textual messages and files. Files are indexed by computing their Tiger Tree Hash (TTH), provided by users through their file list, and searchable on a hub-basis. There exist two implementations, the traditional NMDC protocol (NeoModus Direct Connect) and the newer ADC protocol (Advanced Direct Connect).

## Features

* ADC and NMDC transparent protocol support
* **Active** and **passive** mode
* **Hub**: connection with configurable try count, password authentication, keepalive, compression, encryption
* **Chat**: bidirectional public and private chat
* **File search**: by name or TTH, reply to requests
* **File download**: by name or TTH, full or partial, on ram or disk, multiple in parallel, compression, encryption, configurable download slots, validation via TTH, client fingerprint validation
* **File upload**: upload from personal share, asynchronous file indexing system, file list generation and serving, compression, encryption, configurable upload slots, tthl extension support, client fingerprint validation
* Examples provided for every feature
* Comprehensive test set

The public API can be considered stable.

## Command-line utilities

#### Installation

Go &ge; 1.11 is required. Download, compile and install the binaries:
```
go get github.com/gswly/dctoolkit/cmd/...
```

#### Usage

```
dc-tth [<flags>] <filepath>

Compute the Tiger Tree Hash (TTH) of a given file.

Flags:
  --help  Show context-sensitive help (also try --help-long and
          --help-man).

Args:
  <filepath>  Path to a file
```

```
dc-search --hub=HUB --nick=NICK [<flags>] <query>

Search files and directories by name in a given hub.

Flags:
  --help       Show context-sensitive help (also try
               --help-long and --help-man).
  --hub=HUB    The url of a hub, ie nmdc://hubip:411
  --nick=NICK  The nickname to use
  --pwd=PWD    The password to use
  --passive    Turn on passive mode (ports are not required
               anymore)
  --tcp=3009   The TCP port to use
  --udp=3009   The UDP port to use
  --tls=3010   The TCP-TLS port to use

Args:
  <query>  Search query
```

```
dc-download --hub=HUB --nick=NICK --outdir=OUTDIR [<flags>] <user> <fpath>

Download a file or a directory from a user in a given hub.

Flags:
  --help           Show context-sensitive help (also try
                   --help-long and --help-man).
  --hub=HUB        The url of a hub, ie nmdc://hubip:411
  --nick=NICK      The nickname to use
  --pwd=PWD        The password to use
  --passive        Turn on passive mode (ports are not
                   required anymore)
  --tcp=3009       The TCP port to use
  --udp=3009       The UDP port to use
  --tls=3010       The TCP-TLS port to use
  --outdir=OUTDIR  The directory in which files will be saved

Args:
  <user>   The user from which to download
  <fpath>  The path of the file or directory to download
```

```
dc-share --hub=HUB --nick=NICK [<flags>] <share>

Share a directory in a given hub.

Flags:
  --help           Show context-sensitive help (also try
                   --help-long and --help-man).
  --hub=HUB        The url of a hub, ie nmdc://hubip:411
  --nick=NICK      The nickname to use
  --pwd=PWD        The password to use
  --passive        Turn on passive mode (ports are not
                   required anymore)
  --tcp=3009       The TCP port to use
  --udp=3009       The UDP port to use
  --tls=3010       The TCP-TLS port to use
  --alias="share"  The alias of the share

Args:
  <share>  The directory to share
```

## Library

#### Installation

When using Go &ge; 1.11 and modules (i.e. there's a go.mod file in your project folder), it is enough to write the library name in the import section of the source files that are referring to it. Go will take care of downloading the needed files:
```go
import (
    ...
    dctk "github.com/gswly/dctoolkit"
)
```

When using an older Go version, or modules are not deployed, the library must be downloaded manually:
```
go get github.com/gswly/dctoolkit
```

#### Examples

* [connection_active](example/connection_active.go)
* [connection_passive](example/connection_passive.go)
* [chat_public](example/chat_public.go)
* [chat_private](example/chat_private.go)
* [search](example/search.go)
* [share](example/share.go)
* [magnet](example/magnet.go)
* [download_list](example/download_list.go)
* [download_all_lists](example/download_all_lists.go)
* [download_file](example/download_file.go)
* [download_file_on_disk](example/download_file_on_disk.go)
* [download_file_from_search](example/download_file_from_search.go)
* [download_file_from_list](example/download_file_from_list.go)
* [download_directory_from_list](example/download_directory_from_list.go)

#### Documentation

https://godoc.org/github.com/gswly/dctoolkit

#### Testing

If you want to edit this library and test the results, you can you automated tests through:
```
./test.sh --all
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
