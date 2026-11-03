import type { GroupLine, TeamAgentView } from "../generated/desktopContract.generated";
import type { Item } from "./useController";
import type { Translator } from "./i18n";

type Entry={line:GroupLine;label:string;icon:string;eventText:string;item:Item};
const eventKeys={passed:"team.event.passed",missed:"team.event.missed",working:"team.event.working",finished:"team.event.finished",failed:"team.event.failed",paused:"team.event.paused",resumed:"team.event.resumed",stopped:"team.event.stopped",cut:"team.event.cut",memory:"team.event.memory"} as const;
export class TeamGroupProjection {
  private entries=new Map<number,Entry>();
  private items:Item[]=[];
  update(group:string,lines:readonly GroupLine[],members:readonly TeamAgentView[],t:Translator):Item[] {
    const next=new Map<number,Entry>();
    const items=lines.map(line=>{
      const member=members.find(member=>member.id===line.from);
      const name=line.source==="scheduler"?t("team.scheduler"):line.from==="user"?t("team.you"):member?.name??line.from;
      const label=line.late?`${name} · ${t("team.late")}`:name;
      const icon=`${member?.avatar.glyph??""}\u0000${member?.avatar.color??""}\u0000${member?.avatar.image??""}`;
      const eventKey=line.event&&Object.prototype.hasOwnProperty.call(eventKeys,line.event)?eventKeys[line.event as keyof typeof eventKeys]:undefined;
      const eventText=(eventKey?t(eventKey):line.event??"")+(line.event==="memory"&&line.text?` · ${line.text}`:"");
      const prior=this.entries.get(line.seq);
      if(prior?.line===line&&prior.label===label&&prior.icon===icon&&prior.eventText===eventText){next.set(line.seq,prior);return prior.item;}
      const id=`team:${group}:${line.seq}`;
      const speaker={id:line.from,name:label,glyph:member?.avatar.glyph,color:member?.avatar.color,image:member?.avatar.image};
      const item:Item=line.kind==="post"?(line.from==="user"?{kind:"user",id,messageId:id,turnId:id,text:line.text??"",createdAt:Date.parse(line.at),speaker}
        :{kind:"assistant",id,turnId:id,text:line.text??"",reasoning:"",streaming:false,turnFinal:true,createdAt:Date.parse(line.at),speaker})
        :{kind:"notice",id,turnId:id,level:line.event==="failed"||line.event==="missed"?"warn":"info",text:`${label} · ${eventText}${line.code?` (${line.code})`:""}`,code:`team.${line.event??"event"}`};
      next.set(line.seq,{line,label,icon,eventText,item});return item;
    });
    this.entries=next;
    if(items.length!==this.items.length||items.some((item,index)=>item!==this.items[index]))this.items=items;
    return this.items;
  }
}
