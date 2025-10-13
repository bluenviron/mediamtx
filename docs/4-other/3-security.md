# Security

## Security of released binaries

Binaries published in the [Releases](https://github.com/bluenviron/mediamtx/releases) section of GitHub are the output of a process which has been designed with a security-first approach. Every step from source code to the intended final destination of binaries is fully visible, immune from external interference and can be independently verified.

This is the process:

1. During every release, the [Release workflow](https://github.com/bluenviron/mediamtx/actions/workflows/release.yml) is triggered on GitHub.

2. The release workflow pulls the source code and builds binaries.

3. The release workflow computes SHA256 checksums of binaries and publishes them on the Sigstore Public Good Instance through [GitHub Attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations).

4. Checksums and binaries are published on the Release page.

5. Binaries are downloaded by users to the intended final destination.

It is possible to verify that SHA256 checksums of binaries correspond to the one published on Sigstore by running:

```sh
ls mediamtx_* | xargs -L1 gh attestation verify --repo bluenviron/mediamtx
```

It is possible to verify that binaries have not been altered during transfer from GitHub to the final destination by downloading `checksums.sha256` and running:

```sh
cat checksums.sha256 | grep "$(ls mediamtx_*)" | sha256sum --check
```

## Reporting vulnerabilities

Vulnerabilities can be reported privately by using the [Security Advisory](https://github.com/bluenviron/mediamtx/security/advisories/new) feature of GitHub.
