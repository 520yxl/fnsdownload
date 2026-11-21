import {mkdir,readFile,writeFile,chmod} from "node:fs/promises";
import path from "node:path";
import {randomUUID} from "node:crypto";
const root=process.env.DATA_DIR||"/data", configPath=path.join(root,"config.json"), tasksPath=path.join(root,"tasks.json");
export async function ensureStore(){
  await mkdir(root,{recursive:true,mode:0o700});await mkdir(path.join(root,"sdk"),{recursive:true,mode:0o700});
  try{await readFile(configPath,"utf8");}catch{await saveConfig({appVersion:"0.1.0",deviceId:randomUUID(),adminPasswordHash:null,authBaseUrl:"",productId:"moxyi-download",productSecret:"",thunderAppId:"",downloadDir:process.env.DOWNLOAD_DIR||"/downloads"});}
  try{await readFile(tasksPath,"utf8");}catch{await writeFile(tasksPath,"[]",{mode:0o600});}
}
export async function loadConfig(){return JSON.parse(await readFile(configPath,"utf8"));}
export async function saveConfig(v){await writeFile(configPath,JSON.stringify(v,null,2),{mode:0o600});await chmod(configPath,0o600);}
export async function loadTasks(){try{return JSON.parse(await readFile(tasksPath,"utf8"));}catch{return [];}}
export async function saveTasks(v){await writeFile(tasksPath,JSON.stringify(v,null,2),{mode:0o600});}
export function sdkConfigPath(){return path.join(root,"sdk");}
