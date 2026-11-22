export interface LifecycleNotice {
  id:string;kind:"access_ended"|"removal_soon";dueAt:string;removalAt:string
  emailState:"pending"|"sending"|"sent"|"failed"|"unknown"|"unavailable";attempts:number;operatorSignal:boolean
}
export interface LifecycleStatus {
  notices:LifecycleNotice[]
  cleanup:{operationId:string;kind:"profile"|"account";state:"fenced_pending"|"failed"|"active_content_removed";fencedAt:string;removedAt?:string;recoveryCopies:string;recipients:string;recoveryCopyDeadline?:string;failureCode?:string}[]
  residuals:{operationId:string;class:"recovery_copy"|"recipient";state:string;deadline?:string;overdue:boolean}[]
  purposeCleanup:number
  removalPending:boolean
  contentExpired:boolean
  careerReadAllowed:boolean
  invitationCleanup:{invitationId:string;state:"pending"|"active_payload_removed";fencedAt:string;cleanedAt?:string;recoveryCopyDeadline:string;recoveryCopies:"not_verified"}[]
  nextCheckAt?:string
}
export class LifecycleError extends Error {
  constructor(readonly status:number,message="Lifecycle status unavailable"){super(message)}
}
const record=(v:unknown):v is Record<string,unknown>=>typeof v==="object"&&v!==null&&!Array.isArray(v)
const date=(v:unknown)=>typeof v==="string"&&Number.isFinite(Date.parse(v))
function valid(v:unknown):v is LifecycleStatus{
  if(!record(v)||Object.keys(v).some(k=>!["notices","cleanup","residuals","purposeCleanup","nextCheckAt","removalPending","contentExpired","careerReadAllowed","invitationCleanup"].includes(k))||typeof v.careerReadAllowed!=="boolean"||typeof v.contentExpired!=="boolean"||typeof v.removalPending!=="boolean"||!Number.isSafeInteger(v.purposeCleanup)||Number(v.purposeCleanup)<0||v.nextCheckAt!==undefined&&!date(v.nextCheckAt))return false
  if(!Array.isArray(v.invitationCleanup)||v.invitationCleanup.length>32||!v.invitationCleanup.every(n=>record(n)&&typeof n.invitationId==="string"&&["pending","active_payload_removed"].includes(String(n.state))&&date(n.fencedAt)&&date(n.recoveryCopyDeadline)&&n.recoveryCopies==="not_verified"&&(n.cleanedAt===undefined||date(n.cleanedAt))))return false
  if(!Array.isArray(v.notices)||v.notices.length>32||!v.notices.every(n=>record(n)&&typeof n.id==="string"&&["access_ended","removal_soon"].includes(String(n.kind))&&date(n.dueAt)&&date(n.removalAt)&&["pending","sending","sent","failed","unknown","unavailable"].includes(String(n.emailState))&&Number.isSafeInteger(n.attempts)&&Number(n.attempts)>=0&&Number(n.attempts)<=3&&typeof n.operatorSignal==="boolean"))return false
  if(!Array.isArray(v.cleanup)||v.cleanup.length>32||!v.cleanup.every(n=>record(n)&&typeof n.operationId==="string"&&["profile","account"].includes(String(n.kind))&&["fenced_pending","failed","active_content_removed"].includes(String(n.state))&&date(n.fencedAt)&&n.recoveryCopies==="not_verified"&&n.recipients==="not_verified"&&(n.removedAt===undefined||date(n.removedAt))&&(n.recoveryCopyDeadline===undefined||date(n.recoveryCopyDeadline))))return false
  return Array.isArray(v.residuals)&&v.residuals.length<=64&&v.residuals.every(n=>record(n)&&typeof n.operationId==="string"&&["recovery_copy","recipient"].includes(String(n.class))&&["not_verified","pending","failed","verified_unrecoverable"].includes(String(n.state))&&typeof n.overdue==="boolean"&&(n.deadline===undefined||date(n.deadline)))
}
export async function readLifecycle(accountId:string,signal?:AbortSignal,send:typeof fetch=fetch):Promise<LifecycleStatus>{
  const response=await send("/api/lifecycle/status",{method:"GET",credentials:"same-origin",cache:"no-store",headers:{"X-CareerOS-Account":accountId},signal})
  if(!response.ok)throw new LifecycleError(response.status)
  const value:unknown=await response.json()
  if(!valid(value))throw new LifecycleError(502,"Invalid lifecycle status")
  return value
}
