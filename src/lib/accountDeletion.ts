import { workflowHeaders } from "./workflowRequest"

export type DeletionKind = "profile" | "account"
export interface DeletionStatus {
  operationId: string
  kind: DeletionKind
  state: "fenced_pending" | "failed" | "active_content_removed"
  fencedAt: string
  removedAt?: string
  recoveryCopyDeadline: string
  recoveryCopies: "not_verified"
  recipients: "not_verified"
  failureCode?: "active_removal_failed"
}
export class DeletionError extends Error {
  constructor(readonly code: "authentication" | "conflict" | "unavailable" | "uncertain" | "not_found") { super(code) }
}
function validStatus(value: unknown): value is DeletionStatus {
  if (!value || typeof value !== "object") return false
  const v = value as DeletionStatus
  return Object.keys(v).every(k => ["operationId","kind","state","fencedAt","removedAt","recoveryCopyDeadline","recoveryCopies","recipients","failureCode"].includes(k)) &&
    /^[A-Za-z0-9_-]{1,128}$/.test(v.operationId) && ["profile","account"].includes(v.kind) &&
    ["fenced_pending","failed","active_content_removed"].includes(v.state) &&
    Number.isFinite(Date.parse(v.fencedAt)) && Number.isFinite(Date.parse(v.recoveryCopyDeadline)) &&
    Date.parse(v.recoveryCopyDeadline) === Date.parse(v.fencedAt) + 30*24*60*60*1000 &&
    (v.state !== "active_content_removed" || (typeof v.removedAt === "string" && Number.isFinite(Date.parse(v.removedAt)))) &&
    v.recoveryCopies === "not_verified" && v.recipients === "not_verified" &&
    (v.failureCode === undefined || v.failureCode === "active_removal_failed")
}
type StoragePort = Pick<Storage,"getItem"|"setItem"|"removeItem">

// Only this opaque operation pointer is retained. Mount/reload/status cannot
// start deletion, repeat a POST, infer cleanup or erase someone else's data.
export class AccountDeletion {
  pendingOperation: string | null = null
  private sequence = 0
  canCorrect = false
  private get storageKey() { return `careeros.deletion.v1:${this.accountId}` }
  constructor(readonly accountId: string, private storage?: StoragePort, private request: typeof fetch = fetch, private newKey = () => crypto.randomUUID(), private restrictedRights = false) {
    try { const key=storage?.getItem(this.storageKey);if(key&&/^[A-Za-z0-9_-]{1,128}$/.test(key))this.pendingOperation=key } catch { /* Memory pointer remains available. */ }
  }
  cancel() { this.sequence++ }
  private async call(path: string, init: RequestInit): Promise<DeletionStatus> {
    const sequence=this.sequence
    if(this.restrictedRights)path=path.replace("/api/deletion/","/api/rights/deletion/")
    const response=await this.request(path,{...init,credentials:"same-origin",cache:"no-store",headers:{...workflowHeaders(this.accountId),"Content-Type":"application/json"},signal:AbortSignal.timeout(15000)})
    if(sequence!==this.sequence)throw new DeletionError("unavailable")
    if(response.status===401||response.status===403)throw new DeletionError("authentication")
    if(response.status===404&&init.method==="GET"){this.canCorrect=true;throw new DeletionError("not_found")}
    if(response.status===409||response.status===422)throw new DeletionError("conflict")
    if(!response.ok)throw new DeletionError("unavailable")
    const body:unknown=await response.json()
    if(sequence!==this.sequence||!validStatus(body)||(this.pendingOperation!==null&&body.operationId!==this.pendingOperation))throw new DeletionError("unavailable")
    this.pendingOperation=body.operationId
    this.canCorrect=false
    try {this.storage?.setItem(this.storageKey,body.operationId)}catch{/* No content or credentials are stored. */}
    return body
  }
  recover() {
    return this.call(this.pendingOperation ? `/api/deletion/operations/${encodeURIComponent(this.pendingOperation)}` : "/api/deletion/status",{method:"GET"})
  }
  async confirm(input: {kind: DeletionKind,profileId: string,revision: number}): Promise<DeletionStatus> {
    if(this.restrictedRights&&input.kind!=="account")throw new DeletionError("conflict")
    if(this.pendingOperation!==null&&!this.canCorrect)throw new DeletionError("uncertain")
    this.pendingOperation??=this.newKey()
    this.canCorrect=false
    try {this.storage?.setItem(this.storageKey,this.pendingOperation)}catch{/* In-memory identity is still stable. */}
    try {
      return await this.call("/api/deletion/operations",{method:"POST",body:JSON.stringify({...input,operationId:this.pendingOperation,confirm:true})})
    } catch(error) {
      if(error instanceof DeletionError)throw error
      // A lost response is not a second deletion command.
      try {return await this.recover()}catch{throw new DeletionError("uncertain")}
    }
  }
  newOperation() {
    this.sequence++
    this.pendingOperation=null
    this.canCorrect=false
    try{this.storage?.removeItem(this.storageKey)}catch{/* Only opaque metadata remains. */}
  }
}
