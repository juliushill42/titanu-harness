package littlebit

import (
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "math"
 "os"
)
const SchemaVersion = 1
// Path stores row-major bit-packed sign matrices and learned compensation scales.
// U is outputDim x rank; V is inputDim x rank. Bit 1 means +1; bit 0 means -1.
type Path struct {
 USign string `json:"u_sign"`
 VSign string `json:"v_sign"`
 H []float32 `json:"h"`
 G []float32 `json:"g"`
 L []float32 `json:"l"`
}
type Artifact struct {
 SchemaVersion int `json:"schema_version"`
 InputDim int `json:"input_dim"`
 OutputDim int `json:"output_dim"`
 Rank int `json:"rank"`
 Primary Path `json:"primary"`
 Residual *Path `json:"residual,omitempty"`
}
type decodedPath struct { u,v []byte; h,g,l []float32; rank,in,out int }
type Layer struct { Artifact Artifact; primary decodedPath; residual *decodedPath }

func LoadFile(path string) (*Layer,error) {
 b,e:=os.ReadFile(path); if e!=nil{return nil,fmt.Errorf("read artifact: %w",e)}
 return Load(b)
}
func Load(data []byte) (*Layer,error) {
 var a Artifact
 if e:=json.Unmarshal(data,&a);e!=nil{return nil,fmt.Errorf("decode artifact JSON: %w",e)}
 if a.SchemaVersion!=SchemaVersion{return nil,fmt.Errorf("unsupported schema_version %d",a.SchemaVersion)}
 if a.InputDim<1||a.OutputDim<1||a.Rank<1{return nil,errors.New("input_dim, output_dim and rank must be positive")}
 if a.InputDim>1<<20||a.OutputDim>1<<20||a.Rank>1<<20||int64(a.InputDim)*int64(a.Rank)>1<<27||int64(a.OutputDim)*int64(a.Rank)>1<<27{return nil,errors.New("dimensions exceed the 128 Mi-element safety bound")}
 p,e:=decodePath(a.Primary,a.InputDim,a.OutputDim,a.Rank);if e!=nil{return nil,fmt.Errorf("primary path: %w",e)}
 l:=&Layer{Artifact:a,primary:p}
 if a.Residual!=nil { r,err:=decodePath(*a.Residual,a.InputDim,a.OutputDim,a.Rank);if err!=nil{return nil,fmt.Errorf("residual path: %w",err)};l.residual=&r }
 return l,nil
}
func decodePath(p Path,in,out,rank int)(decodedPath,error){
 u,e:=base64.StdEncoding.DecodeString(p.USign);if e!=nil{return decodedPath{},fmt.Errorf("u_sign base64: %w",e)}
 v,e:=base64.StdEncoding.DecodeString(p.VSign);if e!=nil{return decodedPath{},fmt.Errorf("v_sign base64: %w",e)}
 if len(u)!=(out*rank+7)/8{return decodedPath{},fmt.Errorf("u_sign has %d bytes; want %d",len(u),(out*rank+7)/8)}
 if len(v)!=(in*rank+7)/8{return decodedPath{},fmt.Errorf("v_sign has %d bytes; want %d",len(v),(in*rank+7)/8)}
 if len(p.H)!=out||len(p.G)!=in||len(p.L)!=rank{return decodedPath{},fmt.Errorf("scale lengths must be h=%d g=%d l=%d",out,in,rank)}
 for n,vs:=range map[string][]float32{"h":p.H,"g":p.G,"l":p.L}{for i,x:=range vs{if math.IsNaN(float64(x))||math.IsInf(float64(x),0){return decodedPath{},fmt.Errorf("%s[%d] is not finite",n,i)}}}
 return decodedPath{u:u,v:v,h:append([]float32(nil),p.H...),g:append([]float32(nil),p.G...),l:append([]float32(nil),p.L...),rank:rank,in:in,out:out},nil
}
func signAt(b []byte,i int)float32{if b[i>>3]&(byte(1)<<uint(i&7))!=0{return 1};return -1}
// Apply evaluates diag(h) U_sign diag(l) V_sign^T diag(g) x without materializing W.
func(l *Layer)Apply(x []float32)([]float32,error){
 if l==nil{return nil,errors.New("nil LittleBit layer")}
 if len(x)!=l.Artifact.InputDim{return nil,fmt.Errorf("input length %d; want %d",len(x),l.Artifact.InputDim)}
 for i,v:=range x{if math.IsNaN(float64(v))||math.IsInf(float64(v),0){return nil,fmt.Errorf("input[%d] is not finite",i)}}
 y:=make([]float32,l.Artifact.OutputDim);applyPath(y,x,l.primary);if l.residual!=nil{applyPath(y,x,*l.residual)}
 return y,nil
}
func applyPath(y,x []float32,p decodedPath){
 z:=make([]float32,p.rank)
 for j:=0;j<p.rank;j++{var s float32;for i:=0;i<p.in;i++{s+=signAt(p.v,i*p.rank+j)*p.g[i]*x[i]};z[j]=s*p.l[j]}
 for o:=0;o<p.out;o++{var s float32;for j:=0;j<p.rank;j++{s+=signAt(p.u,o*p.rank+j)*z[j]};y[o]+=p.h[o]*s}
}
func(l *Layer)PackedBytes()int64{
 if l==nil{return 0};n:=int64(len(l.primary.u)+len(l.primary.v)+4*(len(l.primary.h)+len(l.primary.g)+len(l.primary.l)))
 if l.residual!=nil{n+=int64(len(l.residual.u)+len(l.residual.v)+4*(len(l.residual.h)+len(l.residual.g)+len(l.residual.l)))};return n
}
func(l *Layer)Metadata()map[string]any{
 if l==nil{return map[string]any{"loaded":false}}
 return map[string]any{"loaded":true,"format":"titanu-littlebit-layer-v1","input_dim":l.Artifact.InputDim,"output_dim":l.Artifact.OutputDim,"rank":l.Artifact.Rank,"residual":l.residual!=nil,"packed_parameter_bytes":l.PackedBytes(),"dense_fp16_parameter_bytes":int64(l.Artifact.InputDim)*int64(l.Artifact.OutputDim)*2}
}
