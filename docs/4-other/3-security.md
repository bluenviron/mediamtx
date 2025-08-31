# Security

## Reporting vulnerabilities

Vulnerabilities can be reported privately by using the [Security Advisory](https://github.com/bluenviron/mediamtx/security/advisories/new) feature of GitHub.

## Binaries

Binaries are compiled from source through the [Release workflow](https://github.com/bluenviron/mediamtx/actions/workflows/release.yml) without human intervention, preventing any external interference.

You can verify that binaries have been produced by the workflow by using [GitHub Attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds):

```sh
ls mediamtx_* | xargs -L1 gh attestation verify --repo bluenviron/mediamtx
```

You can verify the binaries checksum by downloading `checksums.sha256` and running:

```sh
cat checksums.sha256 | grep "$(ls mediamtx_*)" | sha256sum --check
```
