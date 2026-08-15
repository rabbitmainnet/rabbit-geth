# Rabbit Chain — consolidação do vesting em consensus/lqc

Versão do patch: 1.0.0  
Escopo: Reward Locker e finalização de `consensus/lqc`.

## Alterações

- `lqc.distributeRewards` usa diretamente `core/vesting.CreditReward`;
- `lqc.Finalize` executa `core/vesting.ReleaseAllUnlockedRewards` antes do reward;
- `FinalizeAndAssemble` reutiliza `Finalize`, evitando caminhos diferentes entre produção e
  importação;
- `consensus/lqc/rewardlocker.go` deixa de manter estado próprio e encaminha a API antiga para
  `core/vesting`;
- a factory permanece apontando para `consensus/lqcv2` durante esta etapa.

## Testes adicionados

- limites das quatro Eras;
- 70% para producer e 30% para committee configurado;
- 100% para producer quando não existe committee;
- conservação do remainder e de todos os wei;
- criação e persistência do índice canônico de vesting sob EIP-158;
- compatibilidade da API antiga do locker;
- release executado por `Finalize` com `vm.StateDB` encapsulado por tracing.

## Instalação e validação

O laboratório pode continuar rodando porque a engine ativa ainda é `lqcv2`. Depois de extrair
o pacote, executar:

```bash
go test ./core/vesting ./consensus/lqc ./consensus/lqcv2
```

Não reconstruir ou reiniciar o laboratório antes de avaliar o resultado desses testes. A
troca da factory será um patch separado e somente acontecerá depois dessa validação.
