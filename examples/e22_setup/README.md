### How to build?

- VSCode and Go docker devcontainer is used to build this example
- download VSCode
- clone this repo
- install Remote Containers extension (make sure that you installed docker)
- reopen this project in dev container
- if cross-compiling run:
  - run in terminal:  `env GOOS=linux GOARCH=arm go build`
- if you have VSCode on your RPi, just run: `go build`
- run your binary on RPi