// Write a complete, machine-readable report before returning a worker verdict.
// Node stdout is asynchronous when connected to a pipe (as in Go's
// exec.Command). process.exit() immediately after console.log() can truncate
// a large measured trace, turning good QA evidence into invalid JSON.
export async function flushReportJSON(stream, report) {
  const body = JSON.stringify(report, null, 1) + '\n';
  await new Promise((resolve, reject) => {
    stream.write(body, error => error ? reject(error) : resolve());
  });
}
