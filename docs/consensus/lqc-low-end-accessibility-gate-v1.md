# Rabbit LQC — gate de acessibilidade para PCs fracos v1

Status: protocolo de benchmark. Não é uma escolha de algoritmo e não está implementado no consenso.

## Por que este gate existe

O teste Sybil confirmou que uma posição por endereço permite que um controlador transforme milhares de chaves em quase todo o poder de producer, fallbacks e committee. A simulação de tickets contínuos removeu esse ganho gratuito quando o trabalho total permanece fixo.

Isso ainda não prova que a defesa é acessível. Antes de qualquer implementação, a Rabbit Chain precisa medir se uma pessoa com computador simples consegue participar sem GPU, stake, saldo de RAB ou equipamento especializado.

## Garantia possível

Uma blockchain permissionless não consegue garantir "uma chance por pessoa" sem identidade externa. IP, MAC, fingerprint e serial de dispositivo não resolvem esse problema: podem ser compartilhados, trocados, falsificados ou controlados por VPNs, nuvens e fabricantes.

A garantia técnica buscada é:

> dividir o mesmo trabalho entre muitas identidades não aumenta a chance; cada unidade adicional de chance exige trabalho adicional verificável.

IP e reputação de conexão podem limitar abuso no transporte P2P, mas nunca podem decidir producer, fallback, committee ou recompensa.

## Perfil fraco de referência

O primeiro gate usa como referência conservadora:

- 2 núcleos de CPU disponíveis;
- 4 GiB de RAM total;
- nenhum requisito de GPU;
- um worker de mineração;
- até 128 MiB de memória para a prova;
- computador estimado quatro vezes mais lento que a máquina do laboratório.

Essa estimativa não substitui testes físicos. O resultado permanece `PROVISIONAL` até existir execução em pelo menos três classes reais de hardware, incluindo o perfil fraco.

## Protótipo mensurável

O benchmark usa Argon2id v1.3 somente para medir custo de CPU e memória. Ele não propõe ativar Argon2id no consenso. A versão 1.0.3 intercala os perfis em ordem crescente e decrescente, aquece cada perfil, executa cinco rodadas e usa p95 para os gates. Ela também mede operações isoladas com a coleta de memória fora do cronômetro. Isso separa instabilidade criptográfica de pausas causadas por alocação ou GC. Inversões grandes ou variabilidade superior a 35% rejeitam o perfil afetado.

O gate escolhe o menor perfil que seja simultaneamente estável na execução contínua, estável na verificação isolada e compatível com o orçamento. Perfis maiores instáveis são rejeitados individualmente; eles não anulam um perfil menor que tenha passado. O relatório usa `PARTIAL` para deixar essa distinção explícita e continua sendo apenas uma medição local provisória.

O [RFC 9106](https://datatracker.ietf.org/doc/rfc9106/) define a família Argon2 e sua memória configurável. A literatura do próprio RFC diferencia variantes e usos; portanto, uma medição positiva não basta para transformar uma função de derivação de chave em proof of work.

Também deverão ser comparadas implementações independentes antes de uma escolha:

- [RandomX](https://github.com/tevador/randomx), otimizado para CPUs de propósito geral com execução de código aleatório e técnicas memory-hard;
- [Cuckoo Cycle](https://github.com/tromp/cuckoo), uma família memory-bound com verificação rápida.

Essas referências são candidatas de pesquisa, não decisões da Rabbit Chain.

## O que o benchmark mede

Para 8, 16, 32 e 64 MiB, o programa registra:

- milissegundos por tentativa na máquina local;
- estimativa para um PC quatro vezes mais lento;
- tentativas por segundo;
- verificações possíveis dentro de 1 segundo por bloco;
- dificuldade derivada para 80% de chance de encontrar ao menos um ticket em uma época de 1.280 segundos;
- tempo esperado de um ticket;
- custo estimado de produzir mil tickets.

Um perfil local é apenas provisoriamente qualificado quando:

- usa no máximo 128 MiB;
- a tentativa estimada no PC fraco leva no máximo 2 segundos;
- pelo menos 8 provas cabem no orçamento de verificação de 1 segundo;
- o tempo esperado do ticket não ultrapassa a época.

## Gates ainda obrigatórios

Mesmo que o benchmark local passe, a implementação continua proibida até concluir:

1. testes físicos em hardware fraco, intermediário e moderno;
2. comparação entre CPU, GPU, nuvem e hardware especializado;
3. consumo elétrico e aquecimento durante execução prolongada;
4. custo de verificar provas válidas e inválidas sob flood;
5. limites canônicos de tickets por bloco e por época;
6. ajuste determinístico de dificuldade, fronteiras e overflow;
7. grinding, retenção de tickets, reorgs e replay;
8. novo ataque Sybil com até 100.000 identidades;
9. resiliência, transações, recompensas e assinaturas novamente em PASS;
10. revisão independente do desenho e da implementação.

## Regra de lançamento

Este benchmark nunca libera a mainnet. Seu resultado máximo é `PROVISIONAL`. O genesis congelado, `consensus/lqc` e o laboratório em execução não podem ser modificados pela ferramenta.
