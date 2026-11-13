// A machine-readable Playwright report is release-gate evidence. Do not
// return from the worker before Node's stdout pipe has consumed it.
// Using the stream callback handles backpressure and broken pipes without
// dropping the tail of large WebKit input traces.
export async function flushReportJSON(stream, report) {
  const body = JSON.stringify(report, null, 1) + '\n';
  await new Promise((resolve, reject) => {
    stream.write(body, error => error ? reject(error) : resolve());
  });
}
