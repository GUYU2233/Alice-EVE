import {describe,expect,it} from 'vitest';
import {manifestItems,planItems,planMetric,routeNodes} from './marketPlan';

describe('market plan view model',()=>{
 it('aggregates a basket and exposes endpoint nodes',()=>{const p={from:{StationID:10,NameZH:'甲站'},to:{StationID:20,NameZH:'乙站'},items:[{typeId:1,quantity:2,capital:100,volumeM3:4,unitReturn:80,netProfit:60},{typeId:2,quantity:3,capital:90,volumeM3:6,unitReturn:50,netProfit:60}],capital:190,volumeM3:10,netProfit:120,jumps:4,minSecurity:.7};expect(planItems(p)).toHaveLength(2);expect(planMetric(p,'capital')).toBe(190);expect(planMetric(p,'cargo')).toBe(10);expect(planMetric(p,'profit')).toBe(120);expect(routeNodes(p).map(x=>x.action)).toEqual(['BUY','SELL'])});
 it('keeps chain historical loads after final inventory empties',()=>{const item={typeId:34,quantity:5,capital:100,volumeM3:5,netProfit:50};const p={stops:[{hub:{StationID:1},loads:[{item}],cargoAfterM3:5},{hub:{StationID:2},unloads:[{item}],cargoAfterM3:0}],inventory:[],realizedProfit:50,totalJumps:2,minSecurity:.8};expect(planItems(p)).toEqual([item]);expect(manifestItems(p)).toEqual([item]);expect(planMetric(p,'cargo')).toBe(5);expect(planMetric(p,'jumps')).toBe(2);expect(routeNodes(p)).toHaveLength(2)});
 it('builds single nodes carrying station ids',()=>{const p={SourceLocationID:60000001,DestinationLocationID:60000002,TypeID:34,Quantity:1};const nodes=routeNodes(p);expect(nodes[0].stationId).toBe(60000001);expect(nodes[1].stationId).toBe(60000002);expect(manifestItems(p)).toEqual([p])});
 it('does not invent zero jumps for missing route fields',()=>{expect(planMetric({RouteSafetyStatus:'unavailable'},'jumps')).toBeUndefined();expect(planMetric({Jumps:0,RouteSafetyStatus:'ready'},'jumps')).toBe(0)});
});
