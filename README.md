# Reverser

## Config

This is a full illustration of the config file.

```yaml
entrypoints:
    - name: lmstudio
      addr: :8080 # host:port to listen on
      target: 192.168.1.4:1234 # host:port to forward the request to
      rules: [only_swedish] # one of any rules have to apply for the request to be allowed, all conditions in a rule have to apply for the rule to apply

rules:
    - name: only_swedish
      ranges:
          - from: 85.24.194.40
            to: 85.24.194.42
      block: [] # List of ips to block
      allow: [] # list of ips to allow
      as_numbers: [] # List of AS numbers to allow
      countries: [SE] # List of country codes to allow
      continents: [EU] # List of continent codes to allow
```
