import assert from "node:assert/strict"

// Shared native CDP keyboard actions for the two field-validation browser flows.
export function keyboardFlow({call, evaluate, pause, selector}) {
  const key = async (key, code, windowsVirtualKeyCode) => {
    await call("Input.dispatchKeyEvent", {type:"keyDown", key, code, windowsVirtualKeyCode, ...(key === "Enter" ? {text:"\r"} : {})})
    await call("Input.dispatchKeyEvent", {type:"keyUp", key, code, windowsVirtualKeyCode})
  }
  const keyboardFill = async text => {
    await evaluate(`document.querySelector(${JSON.stringify(selector)}).focus(); document.querySelector(${JSON.stringify(selector)}).select()`)
    await call("Input.insertText", {text})
    await pause(60)
  }
  const keyboardSubmit = async target => {
    await evaluate(`document.querySelector(${JSON.stringify(selector)}).focus()`)
    for(let i=0;i<40;i++) {
      await key("Tab", "Tab", 9)
      if(await evaluate(`document.activeElement === (${target})`)) {
        assert.equal(await evaluate(`(${target}).disabled`), false)
        await key("Enter", "Enter", 13)
        return
      }
    }
    throw new Error("Submit button is not keyboard reachable")
  }
  return {keyboardFill, keyboardSubmit}
}
