# Encrypt the configuration

The configuration file can be entirely encrypted for security purposes by using the `crypto_secretbox` function of the NaCL function. An online tool for performing this operation is [available here](https://play.golang.org/p/rX29jwObNe4).

After performing the encryption, put the base64-encoded result into the configuration file, and launch the server with the `MTX_CONFKEY` variable:

```
MTX_CONFKEY=mykey ./mediamtx
```
