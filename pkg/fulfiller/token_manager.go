package fulfiller

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrunner/pkg/logger"
)

// TokenManager handles token configuration and management
type TokenManager struct {
	tokens          map[int]map[TokenType]Token
	tokenAddressMap map[common.Address]TokenType
	logger          logger.Logger
}

// NewTokenManager creates a new token manager
func NewTokenManager(logger logger.Logger) *TokenManager {
	tokens := make(map[int]map[TokenType]Token)
	tokenAddressMap := make(map[common.Address]TokenType)

	// Initialize token map for each chain
	for chainID := range []int{1, 56, 137, 42161, 10} { // Add all supported chains
		tokens[chainID] = make(map[TokenType]Token)
	}

	// Initialize tokens
	initializeTokens(tokens, tokenAddressMap)

	return &TokenManager{
		tokens:          tokens,
		tokenAddressMap: tokenAddressMap,
		logger:          logger,
	}
}

// GetToken returns a token for a given chain and token type
func (tm *TokenManager) GetToken(chainID int, tokenType TokenType) (Token, bool) {
	chainTokens, exists := tm.tokens[chainID]
	if !exists {
		return Token{}, false
	}

	token, exists := chainTokens[tokenType]
	return token, exists
}

// GetTokenTypeFromAddress gets the token type from an address
func (tm *TokenManager) GetTokenTypeFromAddress(address common.Address) TokenType {
	return tm.tokenAddressMap[address]
}

// GetTokensForChain returns all tokens for a given chain
func (tm *TokenManager) GetTokensForChain(chainID int) map[TokenType]Token {
	return tm.tokens[chainID]
}

// GetAllTokens returns all tokens across all chains
func (tm *TokenManager) GetAllTokens() map[int]map[TokenType]Token {
	return tm.tokens
}

// SetTokens sets the tokens map in the token manager
func (tm *TokenManager) SetTokens(tokens map[int]map[TokenType]Token, tokenAddressMap map[common.Address]TokenType) {
	tm.tokens = tokens
	tm.tokenAddressMap = tokenAddressMap
}
