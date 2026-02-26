#include <unistd.h>
int main(int c,char**v){if(c<4)return 1;char*a[]={"python3","/app/gpt2.py",v[1],v[2],v[3],0};execvp(a[0],a);return 1;}
