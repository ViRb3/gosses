# gosses

![e](https://user-images.githubusercontent.com/760637/71449335-790a4200-274a-11ea-80be-4c536fbad8a1.gif)

[![build status](https://github.com/virb3/gosses/workflows/ci/badge.svg)](https://github.com/virb3/gosses/actions)
[![docker build status](https://img.shields.io/docker/cloud/build/virb3/gosses.svg?logo=docker)](https://hub.docker.com/r/virb3/gosses)
[![docker pulls](https://img.shields.io/docker/pulls/virb3/gosses.svg?logo=docker)](https://hub.docker.com/r/virb3/gosses)
[![github downloads](https://img.shields.io/github/downloads/virb3/gosses/total.svg?logo=github)](https://github.com/virb3/gosses/releases)

A fast and simple webserver for your files.

This is a backwards-compatible rewrite of the original [gossa](https://github.com/pldubouilh/gossa)
backend with more robust handling, best practices and performance improvements.

The original [simple UI](https://github.com/pldubouilh/gossa-ui) comes as default, featuring:

- 🔍 files/directories browser & handler
- 📩 drag-and-drop uploader
- 🚀 lightweight and dependency free
- 💾 90s web UI that prints in ms
- 📸 picture browser
- 📽️ video streaming
- ✍️ simple text editor
- ⌨️ keyboard navigation
- 🥂 fast golang static server
- 🔒 easy/secure multi account setup, read-only mode
- ✨ PWA enabled

### Releases

Releases are available on the [release page](https://github.com/virb3/gosses/releases).

### Usage

```sh
% ./gosses --help

% ./gosses -h 192.168.100.33 ~/storage
```

### Shortcuts

Press `Ctrl/Cmd + H` to see all the UI/keyboard shortcuts.

### Docker

Docker images are published to [DockerHub](https://hub.docker.com/r/virb3/gosses). Simple usage:

```sh
% sudo docker run -v ~/LocalDirToShare:/shared -p 8001:8001 virb3/gosses
```

In a do-one-thing-well mindset, HTTPS and authentication has been left to middlewares and proxies.
For additional setup examples, refer to the original [gossa](https://github.com/pldubouilh/gossa) documentation.
