set dotenv-load := true
set dotenv-override := true

run cmd *ARGS:
    @go run ./cmd/{{ cmd }}/ {{ ARGS }}

docker:
    @docker build -t allower:latest . --load

build cmd:
    @echo "Building smol binary for {{ cmd }}..."
    go build -o ./bin/{{ cmd }} -ldflags="-w -s" ./cmd/{{ cmd }}/

brun cmd *ARGS:
    @if [ -f "./bin/{{ cmd }}" ]; then \
        ./bin/{{ cmd }} {{ ARGS }}; \
    else \
        echo "Binary not found. Please run 'just build {{ cmd }}' first."; \
    fi

bdel cmd:
    @if [ -f "./bin/{{ cmd }}" ]; then \
        rm ./bin/{{ cmd }}; \
        echo "Binary {{ cmd }} deleted."; \
    else \
        echo "Binary not found. Nothing to delete."; \
    fi

test folder pkg run=".":
    go test -v -run {{ run }} ./{{ folder }}/{{ pkg }}/

bench folder pkg run=".":
    go test -v -run {{ run }} -bench . ./{{ folder }}/{{ pkg }}/

clean:
    @echo "Removing bin, *.test, *.out"
    @rm -rf bin
    @rm *.test 2> /dev/null || true
    @rm *.out 2> /dev/null || true

[arg('type', pattern='cpu|mem|block|mutex')]
profile folder pkg type run=".":
    go test -v -run="-" -bench="{{ run }}" -{{ type }}profile={{ type }}.out ./{{ folder }}/{{ pkg }}/

[arg('type', pattern='cpu|mem|block|mutex')]
pprof type port="8080":
    go tool pprof -http=:{{ port }} -no_browser {{ type }}.out
