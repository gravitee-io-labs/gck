# Changelog

## [1.3.0](https://github.com/gravitee-io-labs/gck/compare/v1.2.1...v1.3.0) (2026-08-11)


### Features

* add grafana and otel contexts ([ab01865](https://github.com/gravitee-io-labs/gck/commit/ab0186504d3d7fd98f866aed68d4784433df86b6))
* add otel collector context ([2df80cf](https://github.com/gravitee-io-labs/gck/commit/2df80cf5a463dfe02a9a6dc22491199cbfa936f4))

## [1.2.1](https://github.com/gravitee-io-labs/gck/compare/v1.2.0...v1.2.1) (2026-07-16)


### Bug Fixes

* guard from panic when creating without config ([50e7cac](https://github.com/gravitee-io-labs/gck/commit/50e7cacbb84e111e8c939af06a862a39e4a562b6))

## [1.2.0](https://github.com/gravitee-io-labs/gck/compare/v1.1.1...v1.2.0) (2026-07-06)


### Features

* sync in cluster and host dns ([a2abbd5](https://github.com/gravitee-io-labs/gck/commit/a2abbd57c920158dac4e6c633bb9aefbf3be0d89))

## [1.1.1](https://github.com/gravitee-io-labs/gck/compare/v1.1.0...v1.1.1) (2026-06-22)


### Bug Fixes

* **patch:** reuse the cluster's create-time context on upgrade ([72fa94f](https://github.com/gravitee-io-labs/gck/commit/72fa94f18790c937e8e165cea891ed40af0dac6f))

## [1.1.0](https://github.com/gravitee-io-labs/gck/compare/v1.0.2...v1.1.0) (2026-06-17)


### Features

* add gamma context ([efdaf3f](https://github.com/gravitee-io-labs/gck/commit/efdaf3f4323d22defea2fef7e5ba73b2981ca451))

## [1.0.2](https://github.com/gravitee-io-labs/gck/compare/v1.0.1...v1.0.2) (2026-06-12)


### Bug Fixes

* revert image suffix for apim ([be02a36](https://github.com/gravitee-io-labs/gck/commit/be02a360bba58f5fa80b556ad1425fed7e6f4e52))

## [1.0.1](https://github.com/gravitee-io-labs/gck/compare/v1.0.0...v1.0.1) (2026-06-12)


### Bug Fixes

* resolve flag template vars from parent context and replace invalid TLS keys ([40d0a9e](https://github.com/gravitee-io-labs/gck/commit/40d0a9e929db71b07ff4f974abda733db036f115))

## [1.0.0](https://github.com/gravitee-io-labs/gck/compare/v0.18.2...v1.0.0) (2026-06-11)


### ⚠ BREAKING CHANGES

* The project has been renamed to gck (Gravitee Cluster Kit) and moved to the gravitee-io-labs organization.

### Features

* add ai tooling for context maintainers ([aad0dfa](https://github.com/gravitee-io-labs/gck/commit/aad0dfa6a2bb7cb741e928e1fe9f87d9c5a9e095))
* add apim mysql variant ([a49dccd](https://github.com/gravitee-io-labs/gck/commit/a49dccd9834e815ad01b1e0e8ce6a8a11ddb84f3))
* add buildArgs and platform to builds ([9fe1fc5](https://github.com/gravitee-io-labs/gck/commit/9fe1fc56b27ffcc0f53d45d1e348097ccd825507))
* add cloud provider kind integration ([e2bb595](https://github.com/gravitee-io-labs/gck/commit/e2bb59597b2e01e73b7f81c24434898a5a4b697c))
* add component level readiness ([fc785c8](https://github.com/gravitee-io-labs/gck/commit/fc785c8da157aa02b55175f7067a9bbce686f08c))
* add create and delete notes for end users ([d3b2929](https://github.com/gravitee-io-labs/gck/commit/d3b292977a87c75c1821948d2fd57187689d8bb8))
* add default feature for registries ([1d2ccda](https://github.com/gravitee-io-labs/gck/commit/1d2ccda67d81fdeda3b3b0f05c5f432d5aabe979))
* add dry run to patch command ([3f62096](https://github.com/gravitee-io-labs/gck/commit/3f620961739125fe9be73d209c527c0c2aa44eae))
* add edge-stack and redis contexts ([e28b942](https://github.com/gravitee-io-labs/gck/commit/e28b942c2d6e986fef5aaff437e5a8bb32f43b29))
* add enable-redis flag for apim contexts ([0fb0497](https://github.com/gravitee-io-labs/gck/commit/0fb0497255f00aa4877900306ded43bd17f0d3ad))
* add gateway and  dns resolution ([241afc3](https://github.com/gravitee-io-labs/gck/commit/241afc3859f18fe9a82afb8ded0428d0a61a8108))
* add gravitee am contexts ([ecf5e33](https://github.com/gravitee-io-labs/gck/commit/ecf5e332bf2fe4208e1ae6e08f00e5b85fec5760))
* add gravitee apim aio context ([c76b287](https://github.com/gravitee-io-labs/gck/commit/c76b2872aa9333f4554d9b7465b91f1c08f22f3c))
* add Gravitee APIM enterprise Kafka context ([3a08db6](https://github.com/gravitee-io-labs/gck/commit/3a08db667ed914c813cc4021a5ba10e462460fb9))
* add info command for contexts ([f58e462](https://github.com/gravitee-io-labs/gck/commit/f58e46294e300903d0f3c41ec6dcd58a6ff06847))
* add kafka registry ([9e48e4b](https://github.com/gravitee-io-labs/gck/commit/9e48e4b9371cc0eef5778dcb81bb9fa7d2fb820c))
* add license volume mounts to ee/kafka gateway and api ([903924f](https://github.com/gravitee-io-labs/gck/commit/903924f39732d26e179d1ecc85f7ea60cf77a4b9))
* add notes.create with connection info for all contexts ([a2bd8d6](https://github.com/gravitee-io-labs/gck/commit/a2bd8d646feb52dbac1700dc10ca179e29971362))
* add patch command ([773c7ee](https://github.com/gravitee-io-labs/gck/commit/773c7ee171bfb59c83e97cb0dc206969ce304756))
* add pg variant for apim aio ([0890681](https://github.com/gravitee-io-labs/gck/commit/08906811aa5d9c9382b0b97d6569d022845e22a5))
* add sew list command and refactor status and delete ([7569e2f](https://github.com/gravitee-io-labs/gck/commit/7569e2f415aebb70e83113017004bfdd59dd3df7))
* add sew validate registry command ([7381dff](https://github.com/gravitee-io-labs/gck/commit/7381dffd00c275d0eaf5ca59c169f5656089da07))
* add skip-preload flag for create and patch ([b7b9f63](https://github.com/gravitee-io-labs/gck/commit/b7b9f636a8b459cd7900a26647cf685dbc92732c))
* add support for authenticated registries ([af265f2](https://github.com/gravitee-io-labs/gck/commit/af265f227e7bd204bd05fad72c5bda1a3d7c6fe7))
* add templating support to sew context ([5e77c43](https://github.com/gravitee-io-labs/gck/commit/5e77c434489ebf986adbaa881a42b3c6ae3cdce3))
* add vault context ([30cb0cc](https://github.com/gravitee-io-labs/gck/commit/30cb0cc099a12287976707f7e21f31915c687200))
* address component dependencies ([87cd00a](https://github.com/gravitee-io-labs/gck/commit/87cd00aabaef3dc42098e644e9ceab7c9866040b))
* allow context maintainers to add their logo ([f02300d](https://github.com/gravitee-io-labs/gck/commit/f02300d420905b9f64587d5d6a626fb69558695f))
* allow end user to add components ([d819ddf](https://github.com/gravitee-io-labs/gck/commit/d819ddf2adfd3f5ecdffd620ae653388df70d72f))
* allow multiple contexts composition ([cf54ad6](https://github.com/gravitee-io-labs/gck/commit/cf54ad6730133770db1691149689001ac2951c04))
* allow to create k8s secrets from local files and env ([bfa7b02](https://github.com/gravitee-io-labs/gck/commit/bfa7b02e939430cec97da2c5967c121e73af4180))
* allow to define k8s manifests inline ([aba8480](https://github.com/gravitee-io-labs/gck/commit/aba84808675b94204cea6a1dff621a2b9ed4cfae))
* boostrap helm installer implementation ([68ba282](https://github.com/gravitee-io-labs/gck/commit/68ba282e81edfdd635ff1b426556e6edce6dd95c))
* bring context composition ([0fec6c7](https://github.com/gravitee-io-labs/gck/commit/0fec6c781eb5cb81ea3deeaa7bb724c22e878ec6))
* bring local docker build to the developer loop ([1faab7f](https://github.com/gravitee-io-labs/gck/commit/1faab7f8ddc9b73a7c655dd3a199b9a9073a2f51))
* define deps to user define components ([32dec8e](https://github.com/gravitee-io-labs/gck/commit/32dec8e2151d044a1fc60861767c75b4c4c8cd7d))
* enable image pre-loading for patch command ([57fbdc8](https://github.com/gravitee-io-labs/gck/commit/57fbdc8f7223432c3dcab3aaee1767eb7326ccb4))
* enhance and polish registry ([adb16a0](https://github.com/gravitee-io-labs/gck/commit/adb16a0052b79be9509f81a55c83909f58fa42ba))
* expose standalone contexts via NodePort for host access ([4fb14ac](https://github.com/gravitee-io-labs/gck/commit/4fb14ac60b7630d46715e1cb473aa302f301a78b))
* introduce abstract contexts for maintainers ([066ec3c](https://github.com/gravitee-io-labs/gck/commit/066ec3c2c107258fc69e5f985f7f30012de60401))
* introduce context flags for maintainers ([2c9b1de](https://github.com/gravitee-io-labs/gck/commit/2c9b1de32a5a1bc858c71733e5bd2229c5d388f4))
* introduce preload mode and skip option ([2b634cc](https://github.com/gravitee-io-labs/gck/commit/2b634cc6e91eb1d00be0f83b99b394892bcd2c0a))
* leverage docker layer caching with mirrors ([25e9b60](https://github.com/gravitee-io-labs/gck/commit/25e9b60788587634e11454973abeed71fd4a5501))
* leverage docker layer caching with preloading ([1dfbe11](https://github.com/gravitee-io-labs/gck/commit/1dfbe11ab9b79c51df774ec6ab5a086d1efc72aa))
* make apim context flag available for all contexts ([3adfbbf](https://github.com/gravitee-io-labs/gck/commit/3adfbbf8e6d7897dbdddae1ddfc2fa638253aabf))
* make sew available from well knowns pkg managers ([e982c71](https://github.com/gravitee-io-labs/gck/commit/e982c710b8aa3b1b4f3c0642ac842691cc8182f8))
* merge extra port mapping when composing contexts ([1e8896a](https://github.com/gravitee-io-labs/gck/commit/1e8896a41212e5b2062c1840313a5ad4f8dd8379))
* output diff when running dry run for patch ([7aa1e0a](https://github.com/gravitee-io-labs/gck/commit/7aa1e0af580146d22ca92c6b8b77a993006d6e17))
* persist preload registry data across cluster lifecycles ([9f926a7](https://github.com/gravitee-io-labs/gck/commit/9f926a7dbe12bb5d269901301e3acb0ec5dc8957))
* rebrand as Gravitee Cluster Kit ([df3dca0](https://github.com/gravitee-io-labs/gck/commit/df3dca051dd537bb07947758ab4dabaf2b5f30e9))
* restrict tag set and validate registry tags ([a5aa46f](https://github.com/gravitee-io-labs/gck/commit/a5aa46fd6aa1629fc3ca85fe45598373556eaf62))
* support anchor links in doc site ([002c0f6](https://github.com/gravitee-io-labs/gck/commit/002c0f65ab63cb055f5c8a8381af9d6e5d7bafdf))
* support variable overrides in compound contexts ([49f8831](https://github.com/gravitee-io-labs/gck/commit/49f8831af842244c314e1d9c5d6cd84d8de84625))
* support wild card domain for DNS ([a96529a](https://github.com/gravitee-io-labs/gck/commit/a96529a332924a4bbdc1238575eadb63ccfb3317))


### Bug Fixes

* add elastic image to apim aio preloading ([d4e3416](https://github.com/gravitee-io-labs/gck/commit/d4e3416bc235b9f98771043b3d15d2a9ebc767d3))
* expand env vars in fromFile paths before absolute path check ([ce71449](https://github.com/gravitee-io-labs/gck/commit/ce714499ab5a4d3f03bf49615d0b24c5f4fab270))
* fail fast on cluster name collision ([59355a8](https://github.com/gravitee-io-labs/gck/commit/59355a87c729726102fca646980a83bb30bc1c59))
* fix image tag variables in gravitee contexts ([ed1a490](https://github.com/gravitee-io-labs/gck/commit/ed1a490a1781a560e76279617ebb9568311ca50d))
* handle context vars in HTTP resolver ([b82f032](https://github.com/gravitee-io-labs/gck/commit/b82f032471b43e2536a870a3ea4c90c97517d854))
* handle ns create in manifest installer ([1b591ac](https://github.com/gravitee-io-labs/gck/commit/1b591ac04229dcc06da6b4f918847cebc8f9ec31))
* improve CPK lifecycle and DNS introspection ([fc25c6f](https://github.com/gravitee-io-labs/gck/commit/fc25c6f81fa2e436e73e1a61ceb633282e2e9628))
* install helm repos for user defined components ([ba10606](https://github.com/gravitee-io-labs/gck/commit/ba106065da21255ced6b66e69e208577c4063051))
* make all commands context aware ([66eaeaf](https://github.com/gravitee-io-labs/gck/commit/66eaeaf7417b69e3505d859a10cee83ab29b1311))
* make lb routing and dns resolution consistent across runs ([1f3fbc9](https://github.com/gravitee-io-labs/gck/commit/1f3fbc9440d95e3df2bde736f47fcb060b487a85))
* merge named lists by name in deepMergeValues ([33dc3c9](https://github.com/gravitee-io-labs/gck/commit/33dc3c9a1acb1633864af63d48a21f9acd095de1))
* override gateway servers for dbless ([3d3144b](https://github.com/gravitee-io-labs/gck/commit/3d3144b515481d7f21b5c005602b8a725fd37c9d))
* override standalone services to ClusterIP in parent contexts ([057a3a3](https://github.com/gravitee-io-labs/gck/commit/057a3a3f29ad21a3b961619c014e969ae8a15f21))
* re-format go releaser config for packagge publishing ([3bf6780](https://github.com/gravitee-io-labs/gck/commit/3bf6780146c6209412dddbb0d3042744902ef548))
* remove images from aio apim values ([a36c591](https://github.com/gravitee-io-labs/gck/commit/a36c59164dc34890a1e3e769a27aeb201e649b52))
* resolve manifests from http ([0658caf](https://github.com/gravitee-io-labs/gck/commit/0658caff87a212b569d64289887c1158ac280906))
* save cluster state early so delete works after failed installs ([fd919ae](https://github.com/gravitee-io-labs/gck/commit/fd919aec4d0f423680999c613b17e20abec2299b))
* set info.Main.version during release builds ([a72ccf8](https://github.com/gravitee-io-labs/gck/commit/a72ccf84ad537f2d010b121fec198848a023f5a4))
* sew describe panicking on cluster without from ([05770b0](https://github.com/gravitee-io-labs/gck/commit/05770b08a5eaef0e2c1d49be0da8dcac179b570e))
* skip sudo prompt when no CPK process is running ([536f780](https://github.com/gravitee-io-labs/gck/commit/536f780bde3c8f2371aefafff60abd96b6788740))
* slice reallocated when merging components ([047edab](https://github.com/gravitee-io-labs/gck/commit/047edab2bb3f8d43150b78e38c92a560fa2565f1))
* strip root sew.yaml to prevent feature leaking ([11876d3](https://github.com/gravitee-io-labs/gck/commit/11876d3704090670ff09aae8178842a56e3f432a))
* support extended var format in template engine ([c240051](https://github.com/gravitee-io-labs/gck/commit/c2400513c08ef0c3e33baaa52adcb4f45880685b))


### Performance

* add mirrors for gravitee.io/apim/aio ([d781229](https://github.com/gravitee-io-labs/gck/commit/d7812293de18a29cb4758e7201fcfdd5cd0a66fb))
* remove taint from kind control plane ([347fab4](https://github.com/gravitee-io-labs/gck/commit/347fab4a6761dde2485e64dcf987368bd577b0c2))
* wait for node readiness before deploying ([d09491c](https://github.com/gravitee-io-labs/gck/commit/d09491c64074331f71ec6ae2614f901f1a98270f))

## Changelog
