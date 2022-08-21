# Introduction
CLI tool to use the [OVH HTTP API][ovh-api], thanks to
the [Go wrapper][ovh-api-go]. For more, see:

  - [a blog post about ``ovh-do`` and using the OVH API][mb-ovh-do];
  - [OVH API documentation][ovh-api];
  - [source code of the Go wrapper][ovh-api-go-src].

An old version in python with less features is provided in
[``old/python/ovh-do``][gh-mb-py-ovh-do]. Ultimately, [Go][golang] was
favored, despite being less concise, because of:

  - the safety of static typing (especially when using alpha API calls);
  - ability to cross-compile and deploy a single static binary.

[ovh-api]:         https://api.ovh.com/console/
[ovh-api-go]:      https://github.com/ovh/go-ovh
[ovh-api-go-src]:  https://github.com/ovh/go-ovh/tree/master/ovh
[mb-ovh-do]:       https://tales.mbivert.com/on-using-ovh-api/
[gh-mb-py-ovh-do]: https://github.com/mbivert/ovh-tools/blob/master/old/python/ovh-do
[golang]:          https://go.dev/
