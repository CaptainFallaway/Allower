# Reverser

## Config

This is a full illustration of the config file.

```yaml
entrypoints:
    - name: lmstudio
      addr: :8080                    # host:port to listen on
      keepalive: 30s                 # optional - os socket keepalive tunable - default is 30s
      target: 192.168.1.4:1234       # host:port to forward to
      dial_timeout: 30s              # optional - timeout for establishing a connection to the target - default is 30s
      rules: [only_swedish]          # optional - omit or leave empty to allow all requests

rules:
    - name: only_swedish
      # All fields below are optional. Conditions are checked in priority order;
      # the first match wins. See "Rule evaluation" for details.
      allow:                         # always allowed, even if the IP is also in block
          - 1.2.3.4
      block:                         # always denied, unless the IP is also in allow
          - 5.6.7.8
      ranges:
          - from: 85.24.194.40       # inclusive from/to address range ...
            to:   85.24.194.42
          - prefix: 85.24.194.0/25   # ... or a CIDR prefix; one form per entry, not both
      countries: [SE]                # ISO 3166-1 alpha-2 country codes
      continents: [EU]               # continent codes (AF, AN, AS, EU, NA, OC, SA)
      ass:                           # autonomous systems to allow
          - number: AS24429          # exact match; written with the AS prefix
            name: Taobao             # case-insensitive substring of the AS name
            domain: alibabacloud.com # exact match of the AS domain
            # each field is optional and checked independently - see "AS matching"
```

## Rule evaluation

At the **entrypoint** level, rules are evaluated as a logical OR — a request is allowed as soon as any one rule in the list permits it. If the `rules` list is empty or omitted, all requests are allowed.

Within a single **rule**, conditions are short-circuit evaluated in the following order. The first condition that produces a verdict wins; the rest are not checked.

| Priority | Condition | Verdict when matched |
|---|---|---|
| 1 | IP is in `allow` | ✅ allowed — overrides everything, including `block` |
| 2 | IP is in `block` | ❌ denied — overrides all conditions below |
| 3 | IP falls within any entry in `ranges` | ✅ allowed |
| 4 | IP's country code is in `countries` | ✅ allowed |
| 5 | IP's continent code is in `continents` | ✅ allowed |
| 6 | IP's AS matches any entry in `ass` | ✅ allowed |
| — | *(nothing matched)* | ❌ denied |

## AS matching

Each entry in `ass` can carry up to three optional fields. They are each checked independently; any single field that matches is enough to allow the request.

| Field | Match type |
|---|---|
| `number` | Exact match against the AS number (e.g. `AS24429`). Case-sensitive. |
| `domain` | Exact match against the AS domain (e.g. `alibabacloud.com`). Case-sensitive. |
| `name` | Case-insensitive substring match against the AS name (e.g. `Taobao` matches `Taobao (China) Software Co.`). |

Omitting a field means it is not checked — it does **not** act as a wildcard that matches everything.

Multiple entries in the list are evaluated as OR: the IP is allowed if any entry produces a match.
