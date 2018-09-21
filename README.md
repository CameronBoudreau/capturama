# capturama

## How To Use
Accepts requests at `/capture` and generates an image in png format of a specified webpage. A `url` query parameter with an encoded value is always required to specify the page to generate an image from.
An additional `dynamic_size_selector` query parameter with an encoded value can be passed to specify a particular element to generate a png from rather than the entire page.
An example request to generate an image from a specific element on a specific page could look like `host:port/capture?url=http%3A%2F%2Fwww.example.com%2F&dynamic_size_selector=body%20h1`.
