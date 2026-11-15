package main
import("encoding/json";"flag";"fmt";"os";"strconv";"strings";"github.com/juliushill42/titanu-harness/internal/littlebit")
func main(){if len(os.Args)<2{usage()};switch os.Args[1]{case"inspect":inspect(os.Args[2:]);case"apply":apply(os.Args[2:]);default:usage()}}
func usage(){fmt.Fprintln(os.Stderr,"Usage: titanu-littlebit inspect -artifact FILE | apply -artifact FILE -input comma,separated,values");os.Exit(2)}
func load(p string)*littlebit.Layer{l,e:=littlebit.LoadFile(p);if e!=nil{fatal(e)};return l}
func inspect(a []string){f:=flag.NewFlagSet("inspect",flag.ExitOnError);p:=f.String("artifact","","LittleBit layer artifact JSON");_=f.Parse(a);if *p==""{usage()};b,_:=json.MarshalIndent(load(*p).Metadata(),"","  ");fmt.Println(string(b))}
func apply(a []string){f:=flag.NewFlagSet("apply",flag.ExitOnError);p:=f.String("artifact","","LittleBit layer artifact JSON");v:=f.String("input","","comma-separated float32 input vector");_=f.Parse(a);if *p==""||*v==""{usage()};parts:=strings.Split(*v,",");x:=make([]float32,len(parts));for i,s:=range parts{n,e:=strconv.ParseFloat(strings.TrimSpace(s),32);if e!=nil{fatal(fmt.Errorf("input[%d]: %w",i,e))};x[i]=float32(n)};y,e:=load(*p).Apply(x);if e!=nil{fatal(e)};b,_:=json.Marshal(map[string]any{"output":y});fmt.Println(string(b))}
func fatal(e error){fmt.Fprintln(os.Stderr,e);os.Exit(1)}
