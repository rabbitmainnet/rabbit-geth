# Cliente do cadastro permissionless da Rabbit Chain

`rabbit-registry` cria e envia operações do cadastro LQC sem entregar a chave
privada ao RPC. A prova LightHash e a assinatura são calculadas no computador
do participante.

## Fontes de chave

Use exatamente uma das opções:

- `--keystore ARQUIVO --password-file ARQUIVO`: keystore JSON do geth;
- `--key ARQUIVO`: chave ECDSA bruta com 64 caracteres hexadecimais.

Arquivos de senha e chaves brutas precisam estar em um sistema de arquivos
Linux e com permissão `0600`. Senhas e chaves nunca devem ser digitadas como
argumento nem enviadas ao RPC.

## Operações

```bash
build/bin/rabbit-registry \
  --rpc /caminho/do/geth.ipc \
  --keystore /caminho/UTC--... \
  --password-file /caminho/password.txt \
  --action register
```

Depois do cadastro, use `--action heartbeat` para renovar a atividade ou
`--action exit` para sair. `--dry-run` assina e valida sem enviar. O cliente
consulta `lqc_registryParameters` e `lqc_registryParticipant`, rejeita leituras
de heads diferentes, determina a sequência correta e limita a validade ao
máximo aceito pelo consenso.

O RPC recebe apenas a operação pública já assinada: versão, ação, endereço,
sequência, validade, nonce da prova e assinatura.
