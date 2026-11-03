import type { GroupLine, TeamGroupHistoryPage } from "../generated/desktopContract.generated";

export class TeamHistoryWindow {
  private pages:TeamGroupHistoryPage[]=[];
  private resident:readonly GroupLine[]=[];
  readonly maxPages=3;
  get lines():readonly GroupLine[]{return this.resident;}
  get olderCursor(){return this.pages[0]?.olderCursor??"";}
  get newerCursor(){return this.pages[this.pages.length-1]?.newerCursor??"";}
  get snapshot(){return this.pages[0]?.snapshot??"";}
  accept(page:TeamGroupHistoryPage,direction:"older"|"newer"|"latest") {
    if(page.status!=="ready")return false;
    if(direction!=="latest"&&this.snapshot&&this.snapshot!==page.snapshot)return false;
    const previous=new Map(this.lines.map(line=>[line.seq,line]));
    const lines=page.lines.map(line=>{
      const old=previous.get(line.seq);
      return old&&sameLine(old,line)?old:line;
    });
    const next={...page,lines};
    if(direction==="latest")this.pages=[next];
    else if(direction==="older")this.pages=[next,...this.pages].slice(0,this.maxPages);
    else this.pages=[...this.pages,next].slice(-this.maxPages);
    this.resident=this.pages.flatMap(page=>page.lines);
    return true;
  }
}

function sameLine(a:GroupLine,b:GroupLine):boolean {
  return a.seq===b.seq&&a.at===b.at&&a.from===b.from&&a.kind===b.kind&&a.text===b.text&&a.event===b.event&&a.code===b.code&&a.late===b.late&&a.length===b.length&&a.count===b.count&&
    (a.mentions?.length??0)===(b.mentions?.length??0)&&(a.mentions??[]).every((id,index)=>id===b.mentions?.[index]);
}
