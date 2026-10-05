package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.search.DataStoreRecentSearches
import ru.tinyops.turboist.nativeapp.search.RecentSearches
import ru.tinyops.turboist.nativeapp.search.ReplicaSearchRepository
import ru.tinyops.turboist.nativeapp.search.SearchRepository
import javax.inject.Singleton

/**
 * The two seams the search screen depends on.
 *
 * It asks for interfaces so the screen can be driven in a test without a
 * database behind it and without a file on disk — which is the whole reason the
 * behaviour of the screen lives in a plain object rather than in a view model.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class SearchModule {
    @Binds
    @Singleton
    abstract fun bindSearchRepository(repository: ReplicaSearchRepository): SearchRepository

    @Binds
    @Singleton
    abstract fun bindRecentSearches(recent: DataStoreRecentSearches): RecentSearches
}
