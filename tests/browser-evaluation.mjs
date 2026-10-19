// Keep reload synchronization shared across controlled browser suites. Waiting
// for readyState alone can inspect the previous document before reload starts.
export function createBrowserEvaluation(pause) {
  let mainFrameNavigation = 0
  return {
    observe(message) {
      if (
        message.method === "Page.frameNavigated" &&
        !message.params.frame.parentId
      )
        mainFrameNavigation++
    },
    async evaluate(call, expression, options = {}) {
      const previousNavigation = mainFrameNavigation
      const result = await call("Runtime.evaluate", {
        expression,
        returnByValue: true,
        awaitPromise: true,
        ...options,
      })
      if (result.exceptionDetails)
        throw new Error(
          result.exceptionDetails.exception?.description ||
            result.exceptionDetails.text,
        )
      if (expression.includes("location.reload()")) {
        for (
          let attempt = 0;
          mainFrameNavigation === previousNavigation;
          attempt++
        ) {
          if (attempt === 100)
            throw new Error("Reload did not navigate the main frame")
          await pause(20)
        }
      }
      return result.result.value
    },
  }
}
