# Rabbit LQC — fundação portátil dos tickets sequenciais

Status: **FOUNDATION_ONLY — mainnet bloqueada**

Esta etapa define e testa a prova criptográfica portátil. Ela não modifica a
seleção de produtores, fallbacks, committee, recompensas, headers ou snapshots.

## Parâmetros candidatos

- Algoritmo: Argon2id v1.3 por `golang.org/x/crypto/argon2`.
- Memória: 8 MiB por prova.
- Iterações: 1.
- Paralelismo interno da prova: 1.
- Saída: 32 bytes.
- Máximo candidato: 64 tickets por bloco.
- Verificação independente: no máximo 2 workers, 16 MiB simultâneos.

## Vínculos de segurança

Cada prova compromete:

- domínio Rabbit LQC e versão;
- chain ID;
- época e hash-âncora;
- endereço participante;
- sequência da lane;
- prova anterior da mesma lane.

O ticket completo é assinado com secp256k1 recuperável e low-S. Isso impede
replay entre redes/épocas/âncoras, cópia da prova para outra identidade e salto
de sequência. A validação em lote é canônica, limitada e atômica.

## Limites honestos desta etapa

- Argon2id não é uma VDF criptográfica.
- Hardware adicional continua produzindo trabalho proporcionalmente maior.
- Não existe garantia de "uma pessoa por endereço" sem identidade externa.
- A regra que transforma tickets em producer/fallback/committee ainda não foi
  implementada; portanto a vulnerabilidade Sybil da engine ativa ainda existe.
- A referência pura em Go é portátil, mas ainda aloca memória por prova. A
  otimização reutilizável deverá manter equivalência byte a byte.

## Próximo gate

Integrar pool, codec e snapshots de tickets em uma ativação exclusiva de
laboratório. Só depois serão repetidos os ataques Sybil de até 100.000
identidades, forks, reinícios, carga, recompensas e assinaturas.

O genesis congelado permanece byte a byte idêntico.
