# Changelog

## [0.18.2](https://github.com/a-cordier/sew/compare/v0.18.1...v0.18.2) (2026-05-22)


### Bug Fixes

* handle context vars in HTTP resolver ([b82f032](https://github.com/a-cordier/sew/commit/b82f032471b43e2536a870a3ea4c90c97517d854))

## [0.18.1](https://github.com/a-cordier/sew/compare/v0.18.0...v0.18.1) (2026-05-19)


### Bug Fixes

* fix image tag variables in gravitee contexts ([ed1a490](https://github.com/a-cordier/sew/commit/ed1a490a1781a560e76279617ebb9568311ca50d))

## [0.18.0](https://github.com/a-cordier/sew/compare/v0.17.0...v0.18.0) (2026-05-08)


### Features

* enhance and polish registry ([adb16a0](https://github.com/a-cordier/sew/commit/adb16a0052b79be9509f81a55c83909f58fa42ba))

## [0.17.0](https://github.com/a-cordier/sew/compare/v0.16.0...v0.17.0) (2026-05-04)


### Features

* support variable overrides in compound contexts ([49f8831](https://github.com/a-cordier/sew/commit/49f8831af842244c314e1d9c5d6cd84d8de84625))

## [0.16.0](https://github.com/a-cordier/sew/compare/v0.15.0...v0.16.0) (2026-05-02)


### Features

* add gravitee am contexts ([ecf5e33](https://github.com/a-cordier/sew/commit/ecf5e332bf2fe4208e1ae6e08f00e5b85fec5760))

## [0.15.0](https://github.com/a-cordier/sew/compare/v0.14.0...v0.15.0) (2026-05-02)


### Features

* add apim mysql variant ([a49dccd](https://github.com/a-cordier/sew/commit/a49dccd9834e815ad01b1e0e8ce6a8a11ddb84f3))

## [0.14.0](https://github.com/a-cordier/sew/compare/v0.13.0...v0.14.0) (2026-05-02)


### Features

* add templating support to sew context ([5e77c43](https://github.com/a-cordier/sew/commit/5e77c434489ebf986adbaa881a42b3c6ae3cdce3))


### Bug Fixes

* support extended var format in template engine ([c240051](https://github.com/a-cordier/sew/commit/c2400513c08ef0c3e33baaa52adcb4f45880685b))

## [0.13.0](https://github.com/a-cordier/sew/compare/v0.12.0...v0.13.0) (2026-04-30)


### Features

* add support for authenticated registries ([af265f2](https://github.com/a-cordier/sew/commit/af265f227e7bd204bd05fad72c5bda1a3d7c6fe7))

## [0.12.0](https://github.com/a-cordier/sew/compare/v0.11.0...v0.12.0) (2026-04-12)


### Features

* add enable-redis flag for apim contexts ([0fb0497](https://github.com/a-cordier/sew/commit/0fb0497255f00aa4877900306ded43bd17f0d3ad))
* make apim context flag available for all contexts ([3adfbbf](https://github.com/a-cordier/sew/commit/3adfbbf8e6d7897dbdddae1ddfc2fa638253aabf))
* restrict tag set and validate registry tags ([a5aa46f](https://github.com/a-cordier/sew/commit/a5aa46fd6aa1629fc3ca85fe45598373556eaf62))


### Bug Fixes

* override gateway servers for dbless ([3d3144b](https://github.com/a-cordier/sew/commit/3d3144b515481d7f21b5c005602b8a725fd37c9d))

## [0.11.0](https://github.com/a-cordier/sew/compare/v0.10.0...v0.11.0) (2026-04-12)


### Features

* add vault context ([30cb0cc](https://github.com/a-cordier/sew/commit/30cb0cc099a12287976707f7e21f31915c687200))

## [0.10.0](https://github.com/a-cordier/sew/compare/v0.9.1...v0.10.0) (2026-04-11)


### Features

* add buildArgs and platform to builds ([9fe1fc5](https://github.com/a-cordier/sew/commit/9fe1fc56b27ffcc0f53d45d1e348097ccd825507))
* add edge-stack and redis contexts ([e28b942](https://github.com/a-cordier/sew/commit/e28b942c2d6e986fef5aaff437e5a8bb32f43b29))


### Bug Fixes

* fail fast on cluster name collision ([59355a8](https://github.com/a-cordier/sew/commit/59355a87c729726102fca646980a83bb30bc1c59))

## [0.9.1](https://github.com/a-cordier/sew/compare/v0.9.0...v0.9.1) (2026-03-26)


### Bug Fixes

* re-format go releaser config for packagge publishing ([3bf6780](https://github.com/a-cordier/sew/commit/3bf6780146c6209412dddbb0d3042744902ef548))

## [0.9.0](https://github.com/a-cordier/sew/compare/v0.8.0...v0.9.0) (2026-03-26)


### Features

* make sew available from well knowns pkg managers ([e982c71](https://github.com/a-cordier/sew/commit/e982c710b8aa3b1b4f3c0642ac842691cc8182f8))

## [0.8.0](https://github.com/a-cordier/sew/compare/v0.7.0...v0.8.0) (2026-03-26)


### Features

* add info command for contexts ([f58e462](https://github.com/a-cordier/sew/commit/f58e46294e300903d0f3c41ec6dcd58a6ff06847))
* bring local docker build to the developer loop ([1faab7f](https://github.com/a-cordier/sew/commit/1faab7f8ddc9b73a7c655dd3a199b9a9073a2f51))

## [0.7.0](https://github.com/a-cordier/sew/compare/v0.6.0...v0.7.0) (2026-03-26)


### Features

* introduce preload mode and skip option ([2b634cc](https://github.com/a-cordier/sew/commit/2b634cc6e91eb1d00be0f83b99b394892bcd2c0a))


### Performance

* wait for node readiness before deploying ([d09491c](https://github.com/a-cordier/sew/commit/d09491c64074331f71ec6ae2614f901f1a98270f))

## [0.6.0](https://github.com/a-cordier/sew/compare/v0.5.0...v0.6.0) (2026-03-25)


### Features

* add skip-preload flag for create and patch ([b7b9f63](https://github.com/a-cordier/sew/commit/b7b9f636a8b459cd7900a26647cf685dbc92732c))
* persist preload registry data across cluster lifecycles ([9f926a7](https://github.com/a-cordier/sew/commit/9f926a7dbe12bb5d269901301e3acb0ec5dc8957))


### Bug Fixes

* sew describe panicking on cluster without from ([05770b0](https://github.com/a-cordier/sew/commit/05770b08a5eaef0e2c1d49be0da8dcac179b570e))

## [0.5.0](https://github.com/a-cordier/sew/compare/v0.4.1...v0.5.0) (2026-03-24)


### Features

* add sew list command and refactor status and delete ([7569e2f](https://github.com/a-cordier/sew/commit/7569e2f415aebb70e83113017004bfdd59dd3df7))

## [0.4.1](https://github.com/a-cordier/sew/compare/v0.4.0...v0.4.1) (2026-03-23)


### Bug Fixes

* set info.Main.version during release builds ([a72ccf8](https://github.com/a-cordier/sew/commit/a72ccf84ad537f2d010b121fec198848a023f5a4))

## [0.4.0](https://github.com/a-cordier/sew/compare/v0.3.1...v0.4.0) (2026-03-23)


### Features

* add sew validate registry command ([7381dff](https://github.com/a-cordier/sew/commit/7381dffd00c275d0eaf5ca59c169f5656089da07))
* introduce context flags for maintainers ([2c9b1de](https://github.com/a-cordier/sew/commit/2c9b1de32a5a1bc858c71733e5bd2229c5d388f4))

## [0.3.1](https://github.com/a-cordier/sew/compare/v0.3.0...v0.3.1) (2026-03-21)


### Documentation

* fix broken link in registry section ([6041cf4](https://github.com/a-cordier/sew/commit/6041cf42ac4c1d99e282e07198822bcaf553b10e))
* fix links in maintainer section of readme ([be90647](https://github.com/a-cordier/sew/commit/be90647598dc89039111604d541952e1b9af1d05))
* make gravitee context stick on top in registry ([d462531](https://github.com/a-cordier/sew/commit/d462531b26e86ffbed2c8e29f69c6b4150ad5292))

## [0.3.0](https://github.com/a-cordier/sew/compare/v0.2.0...v0.3.0) (2026-03-21)


### Features

* support anchor links in doc site ([002c0f6](https://github.com/a-cordier/sew/commit/002c0f65ab63cb055f5c8a8381af9d6e5d7bafdf))

## [0.2.0](https://github.com/a-cordier/sew/compare/v0.1.0...v0.2.0) (2026-03-21)


### Features

* add ai tooling for context maintainers ([aad0dfa](https://github.com/a-cordier/sew/commit/aad0dfa6a2bb7cb741e928e1fe9f87d9c5a9e095))
* add cloud provider kind integration ([e2bb595](https://github.com/a-cordier/sew/commit/e2bb59597b2e01e73b7f81c24434898a5a4b697c))
* add component level readiness ([fc785c8](https://github.com/a-cordier/sew/commit/fc785c8da157aa02b55175f7067a9bbce686f08c))
* add create and delete notes for end users ([d3b2929](https://github.com/a-cordier/sew/commit/d3b292977a87c75c1821948d2fd57187689d8bb8))
* add default feature for registries ([1d2ccda](https://github.com/a-cordier/sew/commit/1d2ccda67d81fdeda3b3b0f05c5f432d5aabe979))
* add dry run to patch command ([3f62096](https://github.com/a-cordier/sew/commit/3f620961739125fe9be73d209c527c0c2aa44eae))
* add gateway and  dns resolution ([241afc3](https://github.com/a-cordier/sew/commit/241afc3859f18fe9a82afb8ded0428d0a61a8108))
* add gravitee apim aio context ([c76b287](https://github.com/a-cordier/sew/commit/c76b2872aa9333f4554d9b7465b91f1c08f22f3c))
* add Gravitee APIM enterprise Kafka context ([3a08db6](https://github.com/a-cordier/sew/commit/3a08db667ed914c813cc4021a5ba10e462460fb9))
* add kafka registry ([9e48e4b](https://github.com/a-cordier/sew/commit/9e48e4b9371cc0eef5778dcb81bb9fa7d2fb820c))
* add license volume mounts to ee/kafka gateway and api ([903924f](https://github.com/a-cordier/sew/commit/903924f39732d26e179d1ecc85f7ea60cf77a4b9))
* add notes.create with connection info for all contexts ([a2bd8d6](https://github.com/a-cordier/sew/commit/a2bd8d646feb52dbac1700dc10ca179e29971362))
* add patch command ([773c7ee](https://github.com/a-cordier/sew/commit/773c7ee171bfb59c83e97cb0dc206969ce304756))
* add pg variant for apim aio ([0890681](https://github.com/a-cordier/sew/commit/08906811aa5d9c9382b0b97d6569d022845e22a5))
* address component dependencies ([87cd00a](https://github.com/a-cordier/sew/commit/87cd00aabaef3dc42098e644e9ceab7c9866040b))
* allow context maintainers to add their logo ([f02300d](https://github.com/a-cordier/sew/commit/f02300d420905b9f64587d5d6a626fb69558695f))
* allow end user to add components ([d819ddf](https://github.com/a-cordier/sew/commit/d819ddf2adfd3f5ecdffd620ae653388df70d72f))
* allow multiple contexts composition ([cf54ad6](https://github.com/a-cordier/sew/commit/cf54ad6730133770db1691149689001ac2951c04))
* allow to create k8s secrets from local files and env ([bfa7b02](https://github.com/a-cordier/sew/commit/bfa7b02e939430cec97da2c5967c121e73af4180))
* allow to define k8s manifests inline ([aba8480](https://github.com/a-cordier/sew/commit/aba84808675b94204cea6a1dff621a2b9ed4cfae))
* boostrap helm installer implementation ([68ba282](https://github.com/a-cordier/sew/commit/68ba282e81edfdd635ff1b426556e6edce6dd95c))
* bring context composition ([0fec6c7](https://github.com/a-cordier/sew/commit/0fec6c781eb5cb81ea3deeaa7bb724c22e878ec6))
* define deps to user define components ([32dec8e](https://github.com/a-cordier/sew/commit/32dec8e2151d044a1fc60861767c75b4c4c8cd7d))
* enable image pre-loading for patch command ([57fbdc8](https://github.com/a-cordier/sew/commit/57fbdc8f7223432c3dcab3aaee1767eb7326ccb4))
* expose standalone contexts via NodePort for host access ([4fb14ac](https://github.com/a-cordier/sew/commit/4fb14ac60b7630d46715e1cb473aa302f301a78b))
* introduce abstract contexts for maintainers ([066ec3c](https://github.com/a-cordier/sew/commit/066ec3c2c107258fc69e5f985f7f30012de60401))
* leverage docker layer caching with mirrors ([25e9b60](https://github.com/a-cordier/sew/commit/25e9b60788587634e11454973abeed71fd4a5501))
* leverage docker layer caching with preloading ([1dfbe11](https://github.com/a-cordier/sew/commit/1dfbe11ab9b79c51df774ec6ab5a086d1efc72aa))
* merge extra port mapping when composing contexts ([1e8896a](https://github.com/a-cordier/sew/commit/1e8896a41212e5b2062c1840313a5ad4f8dd8379))
* output diff when running dry run for patch ([7aa1e0a](https://github.com/a-cordier/sew/commit/7aa1e0af580146d22ca92c6b8b77a993006d6e17))
* support wild card domain for DNS ([a96529a](https://github.com/a-cordier/sew/commit/a96529a332924a4bbdc1238575eadb63ccfb3317))


### Bug Fixes

* add elastic image to apim aio preloading ([d4e3416](https://github.com/a-cordier/sew/commit/d4e3416bc235b9f98771043b3d15d2a9ebc767d3))
* expand env vars in fromFile paths before absolute path check ([ce71449](https://github.com/a-cordier/sew/commit/ce714499ab5a4d3f03bf49615d0b24c5f4fab270))
* handle ns create in manifest installer ([1b591ac](https://github.com/a-cordier/sew/commit/1b591ac04229dcc06da6b4f918847cebc8f9ec31))
* improve CPK lifecycle and DNS introspection ([fc25c6f](https://github.com/a-cordier/sew/commit/fc25c6f81fa2e436e73e1a61ceb633282e2e9628))
* install helm repos for user defined components ([ba10606](https://github.com/a-cordier/sew/commit/ba106065da21255ced6b66e69e208577c4063051))
* make all commands context aware ([66eaeaf](https://github.com/a-cordier/sew/commit/66eaeaf7417b69e3505d859a10cee83ab29b1311))
* make lb routing and dns resolution consistent across runs ([1f3fbc9](https://github.com/a-cordier/sew/commit/1f3fbc9440d95e3df2bde736f47fcb060b487a85))
* merge named lists by name in deepMergeValues ([33dc3c9](https://github.com/a-cordier/sew/commit/33dc3c9a1acb1633864af63d48a21f9acd095de1))
* override standalone services to ClusterIP in parent contexts ([057a3a3](https://github.com/a-cordier/sew/commit/057a3a3f29ad21a3b961619c014e969ae8a15f21))
* remove images from aio apim values ([a36c591](https://github.com/a-cordier/sew/commit/a36c59164dc34890a1e3e769a27aeb201e649b52))
* resolve manifests from http ([0658caf](https://github.com/a-cordier/sew/commit/0658caff87a212b569d64289887c1158ac280906))
* save cluster state early so delete works after failed installs ([fd919ae](https://github.com/a-cordier/sew/commit/fd919aec4d0f423680999c613b17e20abec2299b))
* skip sudo prompt when no CPK process is running ([536f780](https://github.com/a-cordier/sew/commit/536f780bde3c8f2371aefafff60abd96b6788740))
* slice reallocated when merging components ([047edab](https://github.com/a-cordier/sew/commit/047edab2bb3f8d43150b78e38c92a560fa2565f1))
* strip root sew.yaml to prevent feature leaking ([11876d3](https://github.com/a-cordier/sew/commit/11876d3704090670ff09aae8178842a56e3f432a))


### Performance

* add mirrors for gravitee-io/apim/aio ([d781229](https://github.com/a-cordier/sew/commit/d7812293de18a29cb4758e7201fcfdd5cd0a66fb))
* remove taint from kind control plane ([347fab4](https://github.com/a-cordier/sew/commit/347fab4a6761dde2485e64dcf987368bd577b0c2))
